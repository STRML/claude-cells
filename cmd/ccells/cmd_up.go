package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/STRML/claude-cells/internal/daemon"
	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/orchestrator"
	"github.com/STRML/claude-cells/internal/tmux"
)

// runUp starts the tmux session + daemon if not running, then attaches.
func runUp(ctx context.Context, repoID, repoPath, stateDir, runtime string) error {
	socketName := fmt.Sprintf("ccells-%s", repoID)
	client := tmux.NewClient(socketName)
	sessionName := "ccells"

	running, err := client.HasSession(ctx, sessionName)
	if err != nil {
		return err
	}

	if running {
		// Already running — just attach
		return doAttach(client, sessionName)
	}

	// Create new session
	if err := client.NewSession(ctx, sessionName); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Resolve path to ccells binary for keybindings
	ccellsBin, err := os.Executable()
	if err != nil {
		ccellsBin = "ccells" // fallback to PATH
	} else {
		ccellsBin, _ = filepath.Abs(ccellsBin)
	}

	// Configure tmux chrome (status line, pane borders, keybindings)
	if err := client.ConfigureChrome(ctx, sessionName, ccellsBin); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to configure tmux chrome: %v\n", err)
	}

	// Create Docker client and orchestrator for workstream operations
	dockerClient, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}

	gitFactory := func(path string) git.GitClient {
		return git.New(path)
	}
	orch := orchestrator.New(dockerClient, gitFactory, repoPath)

	// Wire action handlers (orchestrator + tmux)
	handlers := &actionHandlers{
		orch:    orch,
		tmux:    client,
		session: sessionName,
	}

	// Start daemon for credential refresh + state reconciliation
	daemonSockPath := filepath.Join(stateDir, "daemon.sock")
	d := daemon.New(daemon.Config{
		SocketPath: daemonSockPath,
		OnCreate:   handlers.handleCreate,
		OnRemove:   handlers.handleRemove,
		OnPause:    handlers.handlePause,
		OnUnpause:  handlers.handleUnpause,
	})
	daemonCtx, daemonCancel := context.WithCancel(ctx)
	var daemonWg sync.WaitGroup
	daemonWg.Add(1)
	go func() {
		defer daemonWg.Done()
		if err := d.Run(daemonCtx); err != nil && daemonCtx.Err() == nil {
			fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
		}
	}()

	// Attach to session (blocks until detach or exit)
	attachErr := doAttach(client, sessionName)

	// Print detach summary after tmux exits
	printDetachSummary(repoID, stateDir)

	// Shut down daemon before closing Docker client to avoid races
	daemonCancel()
	daemonWg.Wait()
	dockerClient.Close()

	return attachErr
}

// doAttach execs into the tmux session, replacing the current process's stdio.
func doAttach(client *tmux.Client, sessionName string) error {
	cmd := client.AttachCommand(sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// printDetachSummary prints a summary after tmux detach.
func printDetachSummary(repoID, stateDir string) {
	info := DetachInfo{
		RepoID: repoID,
	}

	// Check if daemon is still reachable
	daemonSock := filepath.Join(stateDir, "daemon.sock")
	resp, err := sendDaemonRequestWithResponse(daemonSock, "ping", nil)
	if err == nil && resp.OK {
		info.DaemonRunning = true
	}

	fmt.Print(formatDetachSummary(info))
}
