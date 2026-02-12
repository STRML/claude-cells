package daemon

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestDaemonStartStop(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "daemon.sock")

	d := New(Config{
		SocketPath: sockPath,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- d.Run(ctx) }()

	// Wait for socket to appear
	time.Sleep(100 * time.Millisecond)

	// Connect and send ping
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	req := Request{Action: "ping"}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("encode: %v", err)
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Errorf("expected OK response, got error: %s", resp.Error)
	}

	// Stop
	cancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop within 2s")
	}
}

func TestDaemonUnknownAction(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "daemon.sock")

	d := New(Config{
		SocketPath: sockPath,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	req := Request{Action: "nonexistent"}
	json.NewEncoder(conn).Encode(req)

	var resp Response
	json.NewDecoder(conn).Decode(&resp)
	if resp.OK {
		t.Error("expected error for unknown action")
	}
	if resp.Error == "" {
		t.Error("expected error message")
	}
}
