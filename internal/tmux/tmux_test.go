package tmux

import (
	"context"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("ccells-test")
	if c.Socket() != "ccells-test" {
		t.Errorf("expected socket ccells-test, got %s", c.Socket())
	}
}

func TestServerRunning(t *testing.T) {
	c := NewClient("ccells-test-nonexistent")
	ctx := context.Background()
	running, err := c.ServerRunning(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if running {
		t.Error("expected server not running")
	}
}
