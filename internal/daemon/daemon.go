package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// ReconcileFunc is called periodically to reconcile tmux+Docker state.
// Returns an error only for logging; reconciliation failures are non-fatal.
type ReconcileFunc func(ctx context.Context) error

// PairingProvider abstracts the sync.Pairing struct for the daemon.
// This avoids a direct dependency on internal/sync from the daemon.
type PairingProvider interface {
	IsActive() bool
	Enable(ctx context.Context, branchName, containerID, localPath, previousBranch string) error
	Disable(ctx context.Context) error
	CheckSyncHealth(ctx context.Context) error
	GetState() PairingState
}

// PairingState is the daemon's view of pairing state.
// Mirrors sync.PairingState but defined here to avoid circular imports.
type PairingState struct {
	Active         bool     `json:"active"`
	CurrentBranch  string   `json:"current_branch"`
	PreviousBranch string   `json:"previous_branch"`
	ContainerID    string   `json:"container_id"`
	SyncHealthy    bool     `json:"sync_healthy"`
	Conflicts      []string `json:"conflicts,omitempty"`
	SyncStatusText string   `json:"sync_status_text"`
}

// Config holds daemon configuration.
type Config struct {
	SocketPath          string
	ReconcileInterval   time.Duration   // default 30s
	ReconcileFunc       ReconcileFunc   // nil = skip reconciliation
	Pairing             PairingProvider // nil = pairing disabled
	PairingPollInterval time.Duration   // default 5s

	// Action handlers — called when CLI sends requests via socket.
	// If nil, the corresponding action returns "not configured".
	OnCreate  func(ctx context.Context, branch, prompt, runtime string) error
	OnRemove  func(ctx context.Context, name string) error
	OnPause   func(ctx context.Context, name string) error
	OnUnpause func(ctx context.Context, name string) error
}

// Daemon is the background process managing credentials, state, and tmux hooks.
type Daemon struct {
	config   Config
	listener net.Listener
	wg       sync.WaitGroup
	cancel   context.CancelFunc // set during Run, called on shutdown
}

// New creates a new daemon.
func New(config Config) *Daemon {
	return &Daemon{config: config}
}

// Run starts the daemon and blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	// Clean up stale socket
	os.Remove(d.config.SocketPath)

	listener, err := net.Listen("unix", d.config.SocketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	d.listener = listener

	// Accept loop
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("[daemon] accept: %v", err)
					time.Sleep(100 * time.Millisecond)
					continue
				}
			}
			d.wg.Add(1)
			go func() {
				defer d.wg.Done()
				d.handleConnection(ctx, conn)
			}()
		}
	}()

	// Background reconciliation loop
	if d.config.ReconcileFunc != nil {
		interval := d.config.ReconcileInterval
		if interval == 0 {
			interval = 30 * time.Second
		}
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := d.config.ReconcileFunc(ctx); err != nil {
						log.Printf("[daemon] reconcile: %v", err)
					}
				}
			}
		}()
	}

	// Pairing health poll loop
	if d.config.Pairing != nil {
		pollInterval := d.config.PairingPollInterval
		if pollInterval == 0 {
			pollInterval = 5 * time.Second
		}
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			ticker := time.NewTicker(pollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if d.config.Pairing.IsActive() {
						if err := d.config.Pairing.CheckSyncHealth(ctx); err != nil {
							log.Printf("[daemon] pairing health: %v", err)
						}
					}
				}
			}
		}()
	}

	// Wait for shutdown
	<-ctx.Done()
	listener.Close()
	d.wg.Wait()

	// Clean up socket
	os.Remove(d.config.SocketPath)
	return ctx.Err()
}

func (d *Daemon) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	// Set read deadline to prevent stalled clients from leaking goroutines
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}

	// Clear deadline for processing + response
	conn.SetReadDeadline(time.Time{})

	var req Request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		writeResponse(conn, Response{Error: "invalid request"})
		return
	}

	resp := d.dispatch(ctx, req)
	writeResponse(conn, resp)
}

func (d *Daemon) dispatch(ctx context.Context, req Request) Response {
	switch req.Action {
	case "ping":
		return Response{OK: true}
	case "create":
		return d.handleCreate(ctx, req.Params)
	case "rm":
		return d.handleRemove(ctx, req.Params)
	case "pause":
		return d.handlePause(ctx, req.Params)
	case "unpause":
		return d.handleUnpause(ctx, req.Params)
	case "pair":
		return d.handlePair(ctx, req.Params)
	case "unpair":
		return d.handleUnpair(ctx)
	case "pair-status":
		return d.handlePairStatus()
	case "shutdown":
		if d.cancel != nil {
			d.cancel()
		}
		return Response{OK: true}
	default:
		return Response{Error: fmt.Sprintf("unknown action: %s", req.Action)}
	}
}

// CreateParams holds parameters for the create action.
type CreateParams struct {
	Branch  string `json:"branch"`
	Prompt  string `json:"prompt"`
	Runtime string `json:"runtime"`
}

// WorkstreamParams holds parameters for rm/pause/unpause actions.
type WorkstreamParams struct {
	Name string `json:"name"`
}

// isValidBranchName validates branch names at the daemon level.
func isValidBranchName(branch string) error {
	if branch == "" {
		return fmt.Errorf("branch is required")
	}
	if len(branch) > 200 {
		return fmt.Errorf("branch name too long")
	}
	for _, r := range branch {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '/' || r == '-') {
			return fmt.Errorf("branch name contains invalid character: %q", r)
		}
	}
	if strings.Contains(branch, "..") || strings.Contains(branch, "//") {
		return fmt.Errorf("branch name contains invalid sequence")
	}
	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") ||
		strings.HasPrefix(branch, "-") || strings.HasSuffix(branch, ".lock") {
		return fmt.Errorf("branch name has invalid prefix or suffix")
	}
	return nil
}

func (d *Daemon) handleCreate(ctx context.Context, params json.RawMessage) Response {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var p CreateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{Error: fmt.Sprintf("invalid create params: %v", err)}
	}
	if err := isValidBranchName(p.Branch); err != nil {
		return Response{Error: err.Error()}
	}

	if d.config.OnCreate == nil {
		return Response{Error: "create handler not configured"}
	}
	if err := d.config.OnCreate(ctx, p.Branch, p.Prompt, p.Runtime); err != nil {
		return Response{Error: fmt.Sprintf("create failed: %v", err)}
	}

	data, err := json.Marshal(map[string]string{"status": "created", "branch": p.Branch})
	if err != nil {
		return Response{Error: fmt.Sprintf("marshal response: %v", err)}
	}
	return Response{OK: true, Data: data}
}

func (d *Daemon) handleRemove(ctx context.Context, params json.RawMessage) Response {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var p WorkstreamParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{Error: fmt.Sprintf("invalid rm params: %v", err)}
	}
	if p.Name == "" {
		return Response{Error: "name is required"}
	}

	if d.config.OnRemove == nil {
		return Response{Error: "remove handler not configured"}
	}
	if err := d.config.OnRemove(ctx, p.Name); err != nil {
		return Response{Error: fmt.Sprintf("rm failed: %v", err)}
	}
	return Response{OK: true}
}

func (d *Daemon) handlePause(ctx context.Context, params json.RawMessage) Response {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var p WorkstreamParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{Error: fmt.Sprintf("invalid pause params: %v", err)}
	}
	if p.Name == "" {
		return Response{Error: "name is required"}
	}

	if d.config.OnPause == nil {
		return Response{Error: "pause handler not configured"}
	}
	if err := d.config.OnPause(ctx, p.Name); err != nil {
		return Response{Error: fmt.Sprintf("pause failed: %v", err)}
	}
	return Response{OK: true}
}

// PairParams holds parameters for the pair action.
type PairParams struct {
	Branch         string `json:"branch"`
	ContainerID    string `json:"container_id"`
	LocalPath      string `json:"local_path"`
	PreviousBranch string `json:"previous_branch"`
}

func (d *Daemon) handlePair(ctx context.Context, params json.RawMessage) Response {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if d.config.Pairing == nil {
		return Response{Error: "pairing not configured"}
	}

	var p PairParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{Error: fmt.Sprintf("invalid pair params: %v", err)}
	}
	if p.Branch == "" {
		return Response{Error: "branch is required"}
	}
	if p.ContainerID == "" {
		return Response{Error: "container_id is required"}
	}
	if p.LocalPath == "" {
		return Response{Error: "local_path is required"}
	}

	if err := d.config.Pairing.Enable(ctx, p.Branch, p.ContainerID, p.LocalPath, p.PreviousBranch); err != nil {
		return Response{Error: fmt.Sprintf("pair failed: %v", err)}
	}

	data, err := json.Marshal(d.config.Pairing.GetState())
	if err != nil {
		return Response{Error: fmt.Sprintf("marshal response: %v", err)}
	}
	return Response{OK: true, Data: data}
}

func (d *Daemon) handleUnpair(ctx context.Context) Response {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if d.config.Pairing == nil {
		return Response{Error: "pairing not configured"}
	}

	if err := d.config.Pairing.Disable(ctx); err != nil {
		return Response{Error: fmt.Sprintf("unpair failed: %v", err)}
	}

	return Response{OK: true}
}

func (d *Daemon) handlePairStatus() Response {
	if d.config.Pairing == nil {
		return Response{Error: "pairing not configured"}
	}

	data, err := json.Marshal(d.config.Pairing.GetState())
	if err != nil {
		return Response{Error: fmt.Sprintf("marshal response: %v", err)}
	}
	return Response{OK: true, Data: data}
}

func (d *Daemon) handleUnpause(ctx context.Context, params json.RawMessage) Response {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var p WorkstreamParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{Error: fmt.Sprintf("invalid unpause params: %v", err)}
	}
	if p.Name == "" {
		return Response{Error: "name is required"}
	}

	if d.config.OnUnpause == nil {
		return Response{Error: "unpause handler not configured"}
	}
	if err := d.config.OnUnpause(ctx, p.Name); err != nil {
		return Response{Error: fmt.Sprintf("unpause failed: %v", err)}
	}
	return Response{OK: true}
}

func writeResponse(conn net.Conn, resp Response) {
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		log.Printf("[daemon] write response: %v", err)
	}
}
