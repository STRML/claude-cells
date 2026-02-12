package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// testSocketPath returns a short socket path to avoid macOS 104-char limit.
func testSocketPath(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("/tmp/claude", fmt.Sprintf("daemon-test-%d", os.Getpid()))
	os.MkdirAll(dir, 0755)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, t.Name()+".sock")
}

func TestDaemonStartStop(t *testing.T) {
	sockPath := testSocketPath(t)

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
	sockPath := testSocketPath(t)

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

// helper to send a daemon request and get a response
func daemonRequest(t *testing.T, sockPath string, action string, params interface{}) Response {
	t.Helper()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	req := Request{Action: action}
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		req.Params = data
	}

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("encode: %v", err)
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

func TestDaemonCreateAction(t *testing.T) {
	sockPath := testSocketPath(t)

	d := New(Config{SocketPath: sockPath})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	// Valid create
	resp := daemonRequest(t, sockPath, "create", CreateParams{
		Branch: "feat/auth",
		Prompt: "Add authentication",
	})
	if !resp.OK {
		t.Errorf("expected OK, got error: %s", resp.Error)
	}

	// Create without branch — should fail
	resp = daemonRequest(t, sockPath, "create", CreateParams{
		Prompt: "No branch specified",
	})
	if resp.OK {
		t.Error("expected error for create without branch")
	}
}

func TestDaemonRemoveAction(t *testing.T) {
	sockPath := testSocketPath(t)

	d := New(Config{SocketPath: sockPath})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	resp := daemonRequest(t, sockPath, "rm", WorkstreamParams{Name: "auth-system"})
	if !resp.OK {
		t.Errorf("expected OK, got error: %s", resp.Error)
	}

	resp = daemonRequest(t, sockPath, "rm", WorkstreamParams{})
	if resp.OK {
		t.Error("expected error for rm without name")
	}
}

func TestDaemonPauseUnpauseActions(t *testing.T) {
	sockPath := testSocketPath(t)

	d := New(Config{SocketPath: sockPath})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	resp := daemonRequest(t, sockPath, "pause", WorkstreamParams{Name: "auth"})
	if !resp.OK {
		t.Errorf("pause: expected OK, got error: %s", resp.Error)
	}

	resp = daemonRequest(t, sockPath, "unpause", WorkstreamParams{Name: "auth"})
	if !resp.OK {
		t.Errorf("unpause: expected OK, got error: %s", resp.Error)
	}
}

func TestDaemonReconcileLoop(t *testing.T) {
	sockPath := testSocketPath(t)

	var callCount int32
	var mu sync.Mutex

	d := New(Config{
		SocketPath:        sockPath,
		ReconcileInterval: 50 * time.Millisecond,
		ReconcileFunc: func(ctx context.Context) error {
			mu.Lock()
			callCount++
			mu.Unlock()
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go d.Run(ctx)
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()

	if count < 2 {
		t.Errorf("expected reconcile to be called at least 2 times, got %d", count)
	}

	cancel()
	time.Sleep(100 * time.Millisecond)
}
