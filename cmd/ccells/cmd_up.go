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
	"github.com/STRML/claude-cells/internal/gitproxy"
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

	if !running {
		// Fresh start — create tmux session with chrome

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
	}

	// Start daemon (needed on both fresh start and reattach).
	// The daemon lives only while ccells is attached — it shuts down on detach
	// and restarts on the next attach. This ensures handlers have valid references
	// to the orchestrator and tmux client.
	dockerClient, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}

	gitFactory := func(path string) git.GitClient {
		return git.New(path)
	}
	orch := orchestrator.New(dockerClient, gitFactory, repoPath)

	// Start git proxy server for container git/gh operations.
	// The PR callback updates tmux pane metadata when a PR is created.
	gitProxyServer := gitproxy.NewServer(buildPRCallback(client, sessionName))
	gitProxyServer.SetPushCompleteCallback(func(workstreamID string) {
		log.Printf("[gitproxy] Push complete for %s", workstreamID)
		// TODO: trigger PR status refresh when polling is implemented
	})

	// Wire action handlers (orchestrator + tmux + git proxy)
	handlers := &actionHandlers{
		orch:     orch,
		tmux:     client,
		session:  sessionName,
		gitProxy: gitProxyServer,
	}

	// Start git proxy sockets for existing containers (on reattach).
	// Sockets are cleaned up on detach, so we need to recreate them.
	handlers.startGitProxiesForExistingPanes(ctx)

	// Reconciliation: cross-reference tmux panes with Docker containers every 30s
	reconcileFunc := buildReconcileFunc(client, dockerClient, sessionName)

	// Start daemon for credential refresh + state reconciliation
	daemonSockPath := filepath.Join(stateDir, "daemon.sock")
	d := daemon.New(daemon.Config{
		SocketPath:    daemonSockPath,
		ReconcileFunc: reconcileFunc,
		OnCreate:      handlers.handleCreate,
		OnRemove:      handlers.handleRemove,
		OnPause:       handlers.handlePause,
		OnUnpause:     handlers.handleUnpause,
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

	// Shut down git proxy server, daemon, and Docker client in order
	gitProxyServer.Shutdown()
	daemonCancel()
	daemonWg.Wait()
	dockerClient.Close()

	return attachErr
}

// buildPRCallback creates a callback that updates tmux pane metadata when
// a PR is created via the git proxy. The callback finds the pane associated
// with the container and updates its border to show the PR badge.
func buildPRCallback(tmuxClient *tmux.Client, session string) gitproxy.PRUpdateCallback {
	return func(workstreamID string, prNumber int, prURL string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		panes, err := tmuxClient.ListPanes(ctx, session)
		if err != nil {
			log.Printf("[gitproxy] PR callback: failed to list panes: %v", err)
			return
		}

		for _, p := range panes {
			cn, _ := tmuxClient.GetPaneOption(ctx, p.ID, "@ccells-container")
			if cn != workstreamID {
				continue
			}

			// Update PR metadata on the pane
			tmuxClient.SetPaneOption(ctx, p.ID, "@ccells-pr", prURL)

			// Update border text to include PR badge
			branch, _ := tmuxClient.GetPaneOption(ctx, p.ID, "@ccells-workstream")
			tmuxClient.SetPaneOption(ctx, p.ID, "@ccells-border-text",
				tmux.FormatPaneBorder(branch, "running", prNumber, ""))

			log.Printf("[gitproxy] Updated pane %s with PR #%d", p.ID, prNumber)
			return
		}

		log.Printf("[gitproxy] PR callback: no pane found for container %s", workstreamID)
	}
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

// buildReconcileFunc creates a reconciliation closure that cross-references
// tmux panes with Docker containers and cleans up orphans.
func buildReconcileFunc(tmuxClient *tmux.Client, dockerClient docker.DockerClient, session string) daemon.ReconcileFunc {
	reconciler := &daemon.Reconciler{}

	return func(ctx context.Context) error {
		// Gather tmux pane state
		panes, err := tmuxClient.ListPanes(ctx, session)
		if err != nil {
			return fmt.Errorf("list panes: %w", err)
		}

		var paneStates []daemon.PaneState
		for _, p := range panes {
			ws, _ := tmuxClient.GetPaneOption(ctx, p.ID, "@ccells-workstream")
			cn, _ := tmuxClient.GetPaneOption(ctx, p.ID, "@ccells-container")
			if ws == "" && cn == "" {
				continue // non-ccells pane
			}
			paneStates = append(paneStates, daemon.PaneState{
				PaneID:     p.ID,
				Workstream: ws,
				Container:  cn,
			})
		}

		// Gather Docker container state
		containers, err := dockerClient.ListDockerTUIContainers(ctx)
		if err != nil {
			return fmt.Errorf("list containers: %w", err)
		}

		var containerStates []daemon.ContainerState
		for _, c := range containers {
			containerStates = append(containerStates, daemon.ContainerState{
				ID:      c.ID,
				Name:    c.Name,
				Running: c.State == "running",
			})
		}

		// Reconcile
		result := reconciler.Reconcile(paneStates, containerStates)

		// Handle orphaned panes (container gone, pane still exists)
		for _, p := range result.OrphanedPanes {
			log.Printf("[reconcile] orphaned pane %s (workstream=%s, container=%s) — killing",
				p.PaneID, p.Workstream, p.Container)
			tmuxClient.KillPane(ctx, p.PaneID)
		}

		// Log orphaned containers (container running, no pane) — don't auto-kill,
		// they may be from a create operation in progress
		for _, c := range result.OrphanedContainers {
			log.Printf("[reconcile] orphaned container %s (%s) — no matching pane", c.Name, c.ID[:12])
		}

		return nil
	}
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
