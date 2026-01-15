package docker

import (
	"context"

	"github.com/docker/docker/client"
)

// Client wraps the Docker SDK client with simplified operations.
type Client struct {
	cli *client.Client
}

// NewClient creates a new Docker client using environment defaults.
func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Client{cli: cli}, nil
}

// Ping checks connectivity to the Docker daemon.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// Close releases the Docker client resources.
func (c *Client) Close() error {
	return c.cli.Close()
}

// Raw returns the underlying Docker client for advanced operations.
func (c *Client) Raw() *client.Client {
	if c == nil {
		return nil
	}
	return c.cli
}
