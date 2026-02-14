package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/STRML/claude-cells/internal/daemon"
	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/orchestrator"
	"github.com/STRML/claude-cells/internal/tmux"
	"github.com/STRML/claude-cells/internal/workstream"
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
		attachErr := doAttach(client, sessionName)
		printDetachSummary(repoID, stateDir)
		return attachErr
	}

	// Resolve path to ccells binary for keybindings
	ccellsBin, err := os.Executable()
	if err != nil {
		ccellsBin = "ccells" // fallback to PATH
	} else {
		ccellsBin, _ = filepath.Abs(ccellsBin)
	}

	// Determine initial pane command based on state.
	paneCmd := determinePaneCommand(ccellsBin, stateDir)

	// Create tmux session — with startup command if needed, plain shell otherwise.
	if paneCmd != "" {
		if err := client.NewSessionWithCommand(ctx, sessionName, paneCmd); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
	} else {
		if err := client.NewSession(ctx, sessionName); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
	}

	// Get git branch for status bar
	gitOps := git.New(repoPath)
	branch, _ := gitOps.CurrentBranch(ctx)
	if branch == "" {
		branch = "main"
	}

	// Configure tmux chrome (status line, pane borders, keybindings)
	if err := client.ConfigureChrome(ctx, sessionName, ccellsBin, repoPath, branch); err != nil {
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

	// Wait for daemon socket to be ready before attaching.
	// The subprocess (ccells welcome / ccells create) needs the daemon
	// to be listening before it can send create requests.
	waitForDaemon(daemonSockPath, 5*time.Second)

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

// determinePaneCommand returns the shell command for the initial tmux pane.
// First time (no state file): full welcome screen with intro + keybindings.
// Returning: jump straight to create dialog.
//
// When this function runs, the tmux session is always fresh (HasSession returned
// false). Any workstream entries in the state file are stale — their panes don't
// exist. We always show a dialog so the user can create a workstream.
func determinePaneCommand(ccellsBin, stateDir string) string {
	if !workstream.StateExists(stateDir) {
		return fmt.Sprintf("'%s' welcome; exec \"$SHELL\"", ccellsBin)
	}
	return fmt.Sprintf("'%s' create --interactive; exec \"$SHELL\"", ccellsBin)
}

// waitForDaemon polls for the daemon socket to appear, up to the given timeout.
// This ensures the daemon is ready before the user can interact with subprocesses
// (like welcome/create dialogs) that need to send daemon requests.
func waitForDaemon(sockPath string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			log.Printf("daemon socket ready: %s", sockPath)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	log.Printf("warning: daemon socket not ready after %v: %s", timeout, sockPath)
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
