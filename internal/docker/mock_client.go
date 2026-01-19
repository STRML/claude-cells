package docker

import (
	"context"
	"sync"
)

// MockClient is a mock implementation of DockerClient for testing.
type MockClient struct {
	mu         sync.Mutex
	containers map[string]*mockContainer

	// Configurable behaviors
	PingErr           error
	CreateContainerFn func(ctx context.Context, cfg *ContainerConfig) (string, error)
	ImageExistsFn     func(ctx context.Context, imageName string) (bool, error)
}

type mockContainer struct {
	ID     string
	State  string // "created", "running", "paused", "exited"
	Config *ContainerConfig
}

// NewMockClient creates a new mock Docker client for testing.
func NewMockClient() *MockClient {
	return &MockClient{
		containers: make(map[string]*mockContainer),
	}
}

func (m *MockClient) Ping(ctx context.Context) error {
	return m.PingErr
}

func (m *MockClient) Close() error {
	return nil
}

func (m *MockClient) CreateContainer(ctx context.Context, cfg *ContainerConfig) (string, error) {
	if m.CreateContainerFn != nil {
		return m.CreateContainerFn(ctx, cfg)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id := "mock-" + cfg.Name
	m.containers[id] = &mockContainer{
		ID:     id,
		State:  "created",
		Config: cfg,
	}
	return id, nil
}

func (m *MockClient) StartContainer(ctx context.Context, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.containers[containerID]; ok {
		c.State = "running"
	}
	return nil
}

func (m *MockClient) StopContainer(ctx context.Context, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.containers[containerID]; ok {
		c.State = "exited"
	}
	return nil
}

func (m *MockClient) RemoveContainer(ctx context.Context, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.containers, containerID)
	return nil
}

func (m *MockClient) PauseContainer(ctx context.Context, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.containers[containerID]; ok {
		c.State = "paused"
	}
	return nil
}

func (m *MockClient) UnpauseContainer(ctx context.Context, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.containers[containerID]; ok {
		c.State = "running"
	}
	return nil
}

func (m *MockClient) GetContainerState(ctx context.Context, containerID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.containers[containerID]; ok {
		return c.State, nil
	}
	return "", &containerNotFoundError{containerID}
}

func (m *MockClient) IsContainerRunning(ctx context.Context, containerID string) (bool, error) {
	state, err := m.GetContainerState(ctx, containerID)
	if err != nil {
		return false, err
	}
	return state == "running", nil
}

func (m *MockClient) ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.containers[containerID]; !ok {
		return "", &containerNotFoundError{containerID}
	}
	// Return mock output based on command
	if len(cmd) > 0 {
		switch cmd[0] {
		case "echo":
			if len(cmd) > 1 {
				return cmd[1], nil
			}
		case "which":
			return "/usr/bin/" + cmd[1], nil
		}
	}
	return "mock output", nil
}

func (m *MockClient) SignalProcess(ctx context.Context, containerID, processName, signal string) error {
	return nil
}

func (m *MockClient) ListDockerTUIContainers(ctx context.Context) ([]ContainerInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []ContainerInfo
	for id, c := range m.containers {
		result = append(result, ContainerInfo{
			ID:    id,
			Name:  c.Config.Name,
			State: c.State,
		})
	}
	return result, nil
}

func (m *MockClient) PruneDockerTUIContainers(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for id, c := range m.containers {
		if c.State == "exited" {
			delete(m.containers, id)
			count++
		}
	}
	return count, nil
}

func (m *MockClient) PruneAllDockerTUIContainers(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := len(m.containers)
	m.containers = make(map[string]*mockContainer)
	return count, nil
}

func (m *MockClient) CleanupOrphanedContainers(ctx context.Context, knownContainerIDs []string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	known := make(map[string]bool)
	for _, id := range knownContainerIDs {
		known[id] = true
	}

	count := 0
	for id := range m.containers {
		if !known[id] {
			delete(m.containers, id)
			count++
		}
	}
	return count, nil
}

func (m *MockClient) ImageExists(ctx context.Context, imageName string) (bool, error) {
	if m.ImageExistsFn != nil {
		return m.ImageExistsFn(ctx, imageName)
	}
	// Default: all images exist
	return true, nil
}

// containerNotFoundError for mock
type containerNotFoundError struct {
	id string
}

func (e *containerNotFoundError) Error() string {
	return "container not found: " + e.id
}

// Verify MockClient implements DockerClient
var _ DockerClient = (*MockClient)(nil)
