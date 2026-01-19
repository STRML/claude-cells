package docker

import "context"

// DockerClient defines the interface for Docker operations.
// This allows for easy mocking in tests.
type DockerClient interface {
	// Client lifecycle
	Ping(ctx context.Context) error
	Close() error

	// Container operations
	CreateContainer(ctx context.Context, cfg *ContainerConfig) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string) error
	RemoveContainer(ctx context.Context, containerID string) error
	PauseContainer(ctx context.Context, containerID string) error
	UnpauseContainer(ctx context.Context, containerID string) error
	GetContainerState(ctx context.Context, containerID string) (string, error)
	IsContainerRunning(ctx context.Context, containerID string) (bool, error)
	ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error)
	SignalProcess(ctx context.Context, containerID, processName, signal string) error

	// Container management
	ListDockerTUIContainers(ctx context.Context) ([]ContainerInfo, error)
	PruneDockerTUIContainers(ctx context.Context) (int, error)
	PruneAllDockerTUIContainers(ctx context.Context) (int, error)
	CleanupOrphanedContainers(ctx context.Context, knownContainerIDs []string) (int, error)

	// Image operations
	ImageExists(ctx context.Context, imageName string) (bool, error)
}

// Verify Client implements DockerClient at compile time
var _ DockerClient = (*Client)(nil)
