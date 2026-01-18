package docker

import (
	"context"
	"testing"
	"time"
)

// skipIfDockerUnavailable skips the test if Docker daemon is not accessible.
func skipIfDockerUnavailable(t *testing.T) *Client {
	t.Helper()
	client, err := NewClient()
	if err != nil {
		t.Skipf("Docker client creation failed (Docker may not be available): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		client.Close()
		t.Skipf("Docker daemon not available: %v", err)
	}

	return client
}

func TestNewClient(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		// This is expected to work even without Docker running
		// as it just creates the client object
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	defer client.Close()
}

func TestClient_Ping(t *testing.T) {
	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := client.Ping(ctx)
	if err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestClient_Close(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
