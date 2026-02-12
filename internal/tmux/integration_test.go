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
