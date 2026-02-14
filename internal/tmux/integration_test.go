//go:build integration

package tmux

import (
	"context"
	"testing"
)

func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	c := NewClient("ccells-test-lifecycle")
	defer c.KillServer(ctx)

	// Create session
	err := c.NewSession(ctx, "test-session")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	// Verify running
	running, _ := c.ServerRunning(ctx)
	if !running {
		t.Fatal("server should be running after NewSession")
	}

	// Verify session exists
	exists, _ := c.HasSession(ctx, "test-session")
	if !exists {
		t.Fatal("session should exist")
	}

	// Verify non-existent session
	exists, _ = c.HasSession(ctx, "nonexistent")
	if exists {
		t.Fatal("nonexistent session should not exist")
	}

	// Kill server
	err = c.KillServer(ctx)
	if err != nil {
		t.Fatalf("KillServer failed: %v", err)
	}

	running, _ = c.ServerRunning(ctx)
	if running {
		t.Fatal("server should not be running after KillServer")
	}
}

func TestPaneLifecycle(t *testing.T) {
	ctx := context.Background()
	c := NewClient("ccells-test-panes")
	defer c.KillServer(ctx)

	err := c.NewSession(ctx, "test-panes")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Initial pane exists (session starts with one)
	panes, err := c.ListPanes(ctx, "test-panes")
	if err != nil {
		t.Fatalf("ListPanes: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(panes))
	}

	// Create a second pane running a command
	paneID, err := c.SplitWindow(ctx, "test-panes", "sleep 300")
	if err != nil {
		t.Fatalf("SplitWindow: %v", err)
	}

	// Set metadata
	err = c.SetPaneOption(ctx, paneID, "@ccells-workstream", "test-branch")
	if err != nil {
		t.Fatalf("SetPaneOption: %v", err)
	}

	// Read metadata
	val, err := c.GetPaneOption(ctx, paneID, "@ccells-workstream")
	if err != nil {
		t.Fatalf("GetPaneOption: %v", err)
	}
	if val != "test-branch" {
		t.Errorf("expected test-branch, got %s", val)
	}

	// List panes — should be 2
	panes, err = c.ListPanes(ctx, "test-panes")
	if err != nil {
		t.Fatalf("ListPanes: %v", err)
	}
	if len(panes) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(panes))
	}

	// Kill pane
	err = c.KillPane(ctx, paneID)
	if err != nil {
		t.Fatalf("KillPane: %v", err)
	}

	panes, _ = c.ListPanes(ctx, "test-panes")
	if len(panes) != 1 {
		t.Fatalf("expected 1 pane after kill, got %d", len(panes))
	}
}

func TestPaneLayout(t *testing.T) {
	ctx := context.Background()
	c := NewClient("ccells-test-layout")
	defer c.KillServer(ctx)

	err := c.NewSession(ctx, "test-layout")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Add a couple panes
	_, err = c.SplitWindow(ctx, "test-layout", "sleep 300")
	if err != nil {
		t.Fatalf("SplitWindow 1: %v", err)
	}
	_, err = c.SplitWindow(ctx, "test-layout", "sleep 300")
	if err != nil {
		t.Fatalf("SplitWindow 2: %v", err)
	}

	// Apply tiled layout
	err = c.SelectLayout(ctx, "test-layout", "tiled")
	if err != nil {
		t.Fatalf("SelectLayout: %v", err)
	}

	// Verify 3 panes
	panes, err := c.ListPanes(ctx, "test-layout")
	if err != nil {
		t.Fatalf("ListPanes: %v", err)
	}
	if len(panes) != 3 {
		t.Fatalf("expected 3 panes, got %d", len(panes))
	}
}
