package gitproxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PRUpdateCallback is called when a PR is created via the proxy.
// The callback should update the workstream state with the PR info.
type PRUpdateCallback func(workstreamID string, prNumber int, prURL string)

// PushCompleteCallback is called when a git push completes successfully.
// This can be used to trigger PR status refresh.
type PushCompleteCallback func(workstreamID string)

// CommandExecutor defines the interface for executing commands.
type CommandExecutor interface {
	Execute(ctx context.Context, op Operation, args []string, ws WorkstreamInfo) (*Response, *PRCreateResult)
}

// Server manages git proxy sockets for all containers.
type Server struct {
	mu             sync.RWMutex
	sockets        map[string]*socketHandler // containerID -> handler
	executor       CommandExecutor
	onPRCreated    PRUpdateCallback
	onPushComplete PushCompleteCallback
	baseDir        string // Base directory for sockets
}

// NewServer creates a new git proxy server.
func NewServer(onPRCreated PRUpdateCallback) *Server {
	return &Server{
		sockets:     make(map[string]*socketHandler),
		executor:    NewExecutor(),
		onPRCreated: onPRCreated,
		baseDir:     "/tmp/ccells/gitproxy",
	}
}

// SetPushCompleteCallback sets the callback for successful push events.
func (s *Server) SetPushCompleteCallback(cb PushCompleteCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPushComplete = cb
}

// SetExecutor replaces the executor (for testing).
func (s *Server) SetExecutor(e CommandExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = e
}

// SetBaseDir sets the base directory for sockets (for testing).
func (s *Server) SetBaseDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseDir = dir
}

// socketHandler manages a single socket for a container.
type socketHandler struct {
	listener   net.Listener
	socketPath string
	workstream WorkstreamInfo
	wsMu       sync.RWMutex // Protects workstream field
	server     *Server
	done       chan struct{}
	wg         sync.WaitGroup
}

// shortContainerID safely truncates a container ID for logging.
func shortContainerID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// StartSocket creates and starts a socket for a container.
func (s *Server) StartSocket(ctx context.Context, containerID string, ws WorkstreamInfo) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if socket already exists for this container
	if handler, exists := s.sockets[containerID]; exists {
		return handler.socketPath, nil
	}

	// Create socket directory
	socketDir := filepath.Join(s.baseDir, containerID)
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create socket directory: %w", err)
	}

	socketPath := filepath.Join(socketDir, "git.sock")

	// Remove stale socket if exists
	os.Remove(socketPath)

	// Create Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return "", fmt.Errorf("failed to create socket: %w", err)
	}

	// Set restrictive permissions on socket.
	// Containers run as root (UID 0), so we try to chown to root:root.
	// The socket directory is already container-specific, providing isolation.
	// Use 0660 to allow owner and group access.
	if err := os.Chown(socketPath, 0, 0); err != nil {
		// Chown may fail if we're not root on the host - that's OK,
		// the socket will be owned by the current user which Docker can access.
		log.Printf("[gitproxy] Could not chown socket to root (non-fatal): %v", err)
	}
	if err := os.Chmod(socketPath, 0660); err != nil {
		listener.Close()
		return "", fmt.Errorf("failed to chmod socket: %w", err)
	}

	handler := &socketHandler{
		listener:   listener,
		socketPath: socketPath,
		workstream: ws,
		server:     s,
		done:       make(chan struct{}),
	}

	s.sockets[containerID] = handler

	// Start accepting connections
	handler.wg.Add(1)
	go handler.acceptLoop()

	log.Printf("[gitproxy] Started socket for container %s at %s (branch: %s)",
		shortContainerID(containerID), socketPath, ws.Branch)

	return socketPath, nil
}

// StopSocket stops and removes a socket for a container.
func (s *Server) StopSocket(containerID string) {
	s.mu.Lock()
	handler, exists := s.sockets[containerID]
	if exists {
		delete(s.sockets, containerID)
	}
	s.mu.Unlock()

	if !exists {
		return
	}

	// Signal shutdown and close listener
	close(handler.done)
	handler.listener.Close()

	// Wait for handler to finish
	handler.wg.Wait()

	// Clean up socket file and directory
	os.Remove(handler.socketPath)
	os.Remove(filepath.Dir(handler.socketPath))

	log.Printf("[gitproxy] Stopped socket for container %s", shortContainerID(containerID))
}

// UpdateWorkstream updates the workstream info for a container's socket.
// This is used to update PR number after creation.
func (s *Server) UpdateWorkstream(containerID string, ws WorkstreamInfo) {
	s.mu.RLock()
	handler, exists := s.sockets[containerID]
	s.mu.RUnlock()

	if exists {
		handler.wsMu.Lock()
		handler.workstream = ws
		handler.wsMu.Unlock()
	}
}

// GetSocketPath returns the socket path for a container, if it exists.
func (s *Server) GetSocketPath(containerID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if handler, exists := s.sockets[containerID]; exists {
		return handler.socketPath
	}
	return ""
}

// Shutdown stops all sockets.
func (s *Server) Shutdown() {
	s.mu.Lock()
	containerIDs := make([]string, 0, len(s.sockets))
	for id := range s.sockets {
		containerIDs = append(containerIDs, id)
	}
	s.mu.Unlock()

	for _, id := range containerIDs {
		s.StopSocket(id)
	}
}

// acceptLoop accepts incoming connections on the socket.
func (h *socketHandler) acceptLoop() {
	defer h.wg.Done()

	for {
		conn, err := h.listener.Accept()
		if err != nil {
			select {
			case <-h.done:
				return // Clean shutdown
			default:
				log.Printf("[gitproxy] Accept error: %v", err)
				continue
			}
		}

		// Handle connection in goroutine
		h.wg.Add(1)
		go h.handleConnection(conn)
	}
}

// handleConnection processes a single request from the container.
func (h *socketHandler) handleConnection(conn net.Conn) {
	defer h.wg.Done()
	defer conn.Close()

	// Read request
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		h.sendError(conn, "failed to read request")
		return
	}

	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		h.sendError(conn, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	// Get a snapshot of workstream info under lock
	h.wsMu.RLock()
	ws := h.workstream
	h.wsMu.RUnlock()

	// Log the request
	log.Printf("[gitproxy] %s: %s %v", ws.Branch, req.Operation, req.Args)

	// Validate operation
	if !IsAllowedOperation(req.Operation) {
		h.sendError(conn, fmt.Sprintf("operation not allowed: %s", req.Operation))
		return
	}

	// Validate arguments against workstream constraints
	if err := Validate(req.Operation, req.Args, ws); err != nil {
		h.sendError(conn, err.Error())
		return
	}

	// Execute the command with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	resp, prResult := h.server.executor.Execute(ctx, req.Operation, req.Args, ws)

	// If PR was created, update workstream and notify callback
	if prResult != nil {
		h.wsMu.Lock()
		h.workstream.PRNumber = prResult.Number
		wsID := h.workstream.ID
		h.wsMu.Unlock()

		if h.server.onPRCreated != nil {
			h.server.onPRCreated(wsID, prResult.Number, prResult.URL)
		}
		log.Printf("[gitproxy] PR #%d created for %s", prResult.Number, ws.Branch)
	}

	// If push completed successfully, trigger PR status refresh
	if req.Operation == OpGitPush && resp.ExitCode == 0 {
		h.server.mu.RLock()
		cb := h.server.onPushComplete
		h.server.mu.RUnlock()

		if cb != nil {
			cb(ws.ID)
			log.Printf("[gitproxy] Push complete for %s, triggering PR status refresh", ws.Branch)
		}
	}

	// Send response
	h.sendResponse(conn, resp)
}

// sendError sends an error response.
func (h *socketHandler) sendError(conn net.Conn, msg string) {
	resp := &Response{
		ExitCode: 1,
		Error:    msg,
	}
	h.sendResponse(conn, resp)
}

// sendResponse sends a JSON response.
func (h *socketHandler) sendResponse(conn net.Conn, resp *Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[gitproxy] Failed to marshal response: %v", err)
		return
	}
	data = append(data, '\n')
	conn.Write(data)
}
