package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/STRML/claude-cells/internal/tmux"
)

// runDown stops the daemon and tmux session. With --rm, destroys containers too.
func runDown(ctx context.Context, repoID, stateDir string, destroyContainers bool) error {
	socketName := fmt.Sprintf("ccells-%s", repoID)
	client := tmux.NewClient(socketName)

	// Try to signal daemon to shut down gracefully
	daemonSock := stateDir + "/daemon.sock"
	if err := sendDaemonRequest(daemonSock, "shutdown"); err != nil {
		// Daemon may not be running — that's OK
	}

	// If --rm, send destroy-all request first
	if destroyContainers {
		if err := sendDaemonRequest(daemonSock, "destroy-all"); err != nil {
			// Fall through — we'll still kill the tmux server
		}
	}

	// Kill tmux server
	running, _ := client.HasSession(ctx, "ccells")
	if running {
		if err := client.KillServer(ctx); err != nil {
			return fmt.Errorf("failed to kill tmux server: %w", err)
		}
		fmt.Println("ccells session stopped.")
	} else {
		fmt.Println("No ccells session running.")
	}

	return nil
}

// sendDaemonRequest sends a request to the daemon via Unix socket.
func sendDaemonRequest(sockPath, action string) error {
	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		return fmt.Errorf("daemon not reachable: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	req := struct {
		Action string `json:"action"`
	}{Action: action}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return fmt.Errorf("send: %w", err)
	}

	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("recv: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("daemon error: %s", resp.Error)
	}
	return nil
}
