package main

import (
	"context"
	"fmt"

	"github.com/STRML/claude-cells/internal/orchestrator"
	"github.com/STRML/claude-cells/internal/tmux"
	"github.com/STRML/claude-cells/internal/workstream"
)

// actionHandlers wires daemon actions to the orchestrator + tmux.
type actionHandlers struct {
	orch    orchestrator.WorkstreamOrchestrator
	tmux    *tmux.Client
	session string
}

// handleCreate creates a workstream (container + worktree) and a tmux pane.
func (h *actionHandlers) handleCreate(ctx context.Context, branch, prompt, runtime string) error {
	// Create workstream object
	ws := workstream.New(prompt)
	ws.BranchName = branch // override auto-generated name with user-specified
	ws.Runtime = runtime

	// Create via orchestrator (worktree + container)
	result, err := h.orch.CreateWorkstream(ctx, ws, orchestrator.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create workstream: %w", err)
	}

	// Build docker exec command for the pane.
	// --dangerously-skip-permissions is safe here: containers are isolated environments.
	// This also bypasses the workspace trust dialog ("Is this a project you trust?").
	rt := runtime
	if rt == "" {
		rt = "claude"
	}
	cmd := fmt.Sprintf("docker exec -it %s %s --dangerously-skip-permissions", result.ContainerName, rt)

	// Check if we should respawn the initial empty pane or split
	panes, err := h.tmux.ListPanes(ctx, h.session)
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}

	var paneID string
	if len(panes) == 1 {
		// Check if the only pane is the initial empty one (no workstream metadata)
		wsName, _ := h.tmux.GetPaneOption(ctx, panes[0].ID, "@ccells-workstream")
		if wsName == "" {
			// Initial empty pane — respawn it with the workstream command
			if err := h.tmux.RespawnPane(ctx, panes[0].ID, cmd); err != nil {
				return fmt.Errorf("respawn pane: %w", err)
			}
			paneID = panes[0].ID
		}
	}

	if paneID == "" {
		// Split window for additional panes
		id, err := h.tmux.SplitWindow(ctx, h.session, cmd)
		if err != nil {
			return fmt.Errorf("split window: %w", err)
		}
		paneID = id
		// Rebalance layout
		h.tmux.SelectLayout(ctx, h.session, "tiled")
	}

	// Set pane metadata for identification
	h.tmux.SetPaneOption(ctx, paneID, "@ccells-workstream", branch)
	h.tmux.SetPaneOption(ctx, paneID, "@ccells-container", result.ContainerName)
	h.tmux.SetPaneOption(ctx, paneID, "@ccells-border-text",
		tmux.FormatPaneBorder(branch, "running", 0, ""))

	return nil
}

// handleRemove destroys a workstream and its tmux pane.
func (h *actionHandlers) handleRemove(ctx context.Context, name string) error {
	// Find pane with matching workstream name
	paneID, containerName, err := h.findPane(ctx, name)
	if err != nil {
		return err
	}

	// Kill the tmux pane first
	if err := h.tmux.KillPane(ctx, paneID); err != nil {
		return fmt.Errorf("kill pane: %w", err)
	}

	// Destroy via orchestrator (stop container, remove worktree)
	ws := workstream.New("")
	ws.BranchName = name
	ws.ContainerID = containerName // Docker SDK accepts name or ID
	if err := h.orch.DestroyWorkstream(ctx, ws, orchestrator.DestroyOptions{}); err != nil {
		return fmt.Errorf("destroy workstream: %w", err)
	}

	return nil
}

// handlePause pauses a workstream's container.
func (h *actionHandlers) handlePause(ctx context.Context, name string) error {
	_, containerName, err := h.findPane(ctx, name)
	if err != nil {
		return err
	}

	ws := workstream.New("")
	ws.ContainerID = containerName
	return h.orch.PauseWorkstream(ctx, ws)
}

// handleUnpause resumes a workstream's container and respawns its pane.
func (h *actionHandlers) handleUnpause(ctx context.Context, name string) error {
	paneID, containerName, err := h.findPane(ctx, name)
	if err != nil {
		return err
	}

	ws := workstream.New("")
	ws.ContainerID = containerName
	if err := h.orch.ResumeWorkstream(ctx, ws); err != nil {
		return err
	}

	// Respawn the pane to restart Claude in the container
	rt := "claude" // TODO: store runtime in pane metadata
	cmd := fmt.Sprintf("docker exec -it %s %s --dangerously-skip-permissions --resume", containerName, rt)
	h.tmux.RespawnPane(ctx, paneID, cmd)

	return nil
}

// findPane locates a tmux pane by workstream name, returning pane ID and container name.
func (h *actionHandlers) findPane(ctx context.Context, name string) (paneID, containerName string, err error) {
	panes, err := h.tmux.ListPanes(ctx, h.session)
	if err != nil {
		return "", "", fmt.Errorf("list panes: %w", err)
	}

	for _, p := range panes {
		ws, _ := h.tmux.GetPaneOption(ctx, p.ID, "@ccells-workstream")
		if ws == name {
			cn, _ := h.tmux.GetPaneOption(ctx, p.ID, "@ccells-container")
			if cn == "" {
				return "", "", fmt.Errorf("workstream %q has no container", name)
			}
			return p.ID, cn, nil
		}
	}

	return "", "", fmt.Errorf("workstream %q not found", name)
}
