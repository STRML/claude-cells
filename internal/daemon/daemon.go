package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

// Config holds daemon configuration.
type Config struct {
	SocketPath string
}

// Daemon is the background process managing credentials, state, and tmux hooks.
type Daemon struct {
	config   Config
	listener net.Listener
	wg       sync.WaitGroup
}

// New creates a new daemon.
func New(config Config) *Daemon {
	return &Daemon{config: config}
}

// Run starts the daemon and blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	// Clean up stale socket
	os.Remove(d.config.SocketPath)

	listener, err := net.Listen("unix", d.config.SocketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	d.listener = listener

	// Accept loop
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("[daemon] accept: %v", err)
					continue
				}
			}
			d.wg.Add(1)
			go func() {
				defer d.wg.Done()
				d.handleConnection(ctx, conn)
			}()
		}
	}()

	// Wait for shutdown
	<-ctx.Done()
	listener.Close()
	d.wg.Wait()

	// Clean up socket
	os.Remove(d.config.SocketPath)
	return ctx.Err()
}

func (d *Daemon) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}

	var req Request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		writeResponse(conn, Response{Error: "invalid request"})
		return
	}

	resp := d.dispatch(ctx, req)
	writeResponse(conn, resp)
}

func (d *Daemon) dispatch(ctx context.Context, req Request) Response {
	switch req.Action {
	case "ping":
		return Response{OK: true}
	default:
		return Response{Error: fmt.Sprintf("unknown action: %s", req.Action)}
	}
}

func writeResponse(conn net.Conn, resp Response) {
	json.NewEncoder(conn).Encode(resp)
}
