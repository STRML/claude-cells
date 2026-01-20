package workstream

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// StateRepairResult contains the results of state validation/repair
type StateRepairResult struct {
	// SessionIDsRepaired is the count of session IDs that were missing and repaired
	SessionIDsRepaired int
	// SessionIDsMissing is the count of session IDs that couldn't be found
	SessionIDsMissing int
	// ContainersNotRunning is the count of containers that weren't running
	ContainersNotRunning int
	// Errors contains any errors encountered during repair
	Errors []string
	// RepairedWorkstreams contains the IDs of workstreams that were repaired
	RepairedWorkstreams []string
}

// IsCorrupted returns true if there were any issues that couldn't be fully repaired
func (r *StateRepairResult) IsCorrupted() bool {
	return r.SessionIDsMissing > 0 || len(r.Errors) > 0
}

// WasRepaired returns true if any repairs were made
func (r *StateRepairResult) WasRepaired() bool {
	return r.SessionIDsRepaired > 0
}

// Summary returns a human-readable summary of the repair results
func (r *StateRepairResult) Summary() string {
	var parts []string
	if r.SessionIDsRepaired > 0 {
		parts = append(parts, fmt.Sprintf("%d session ID(s) repaired", r.SessionIDsRepaired))
	}
	if r.SessionIDsMissing > 0 {
		parts = append(parts, fmt.Sprintf("%d session ID(s) could not be recovered", r.SessionIDsMissing))
	}
	if r.ContainersNotRunning > 0 {
		parts = append(parts, fmt.Sprintf("%d container(s) not running", r.ContainersNotRunning))
	}
	if len(r.Errors) > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", len(r.Errors)))
	}
	if len(parts) == 0 {
		return "State is valid"
	}
	return strings.Join(parts, ", ")
}

// sessionIDFromContainerRegex matches session IDs in container output
// Supports both UUIDs (36 chars with dashes) and ULIDs (26 alphanumeric chars)
var sessionIDFromContainerRegex = regexp.MustCompile(`(?:session(?:[_\s]?id)?[:\s]+|Resuming session[:\s]+)([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}|[0-9A-Za-z]{26})`)

// ValidateAndRepairState checks the state for missing session IDs and attempts to repair them
// by querying the running containers for their current Claude session.
func ValidateAndRepairState(ctx context.Context, workstreams []*Workstream) (*StateRepairResult, error) {
	result := &StateRepairResult{}

	// Get Docker client
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerClient.Close()

	for _, ws := range workstreams {
		// Skip if already has session ID
		if ws.GetClaudeSessionID() != "" {
			continue
		}

		// Skip if no container ID
		if ws.ContainerID == "" {
			continue
		}

		// Check if container is running
		containerInfo, err := dockerClient.ContainerInspect(ctx, ws.ContainerID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("container %s: %v", ws.ContainerID[:12], err))
			continue
		}

		if !containerInfo.State.Running {
			result.ContainersNotRunning++
			continue
		}

		// Try to extract session ID from container
		sessionID, err := extractSessionIDFromContainer(ctx, dockerClient, ws.ContainerID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("workstream %s: %v", ws.ID, err))
			result.SessionIDsMissing++
			continue
		}

		if sessionID == "" {
			result.SessionIDsMissing++
			continue
		}

		// Repair: set the session ID
		ws.SetClaudeSessionID(sessionID)
		result.SessionIDsRepaired++
		result.RepairedWorkstreams = append(result.RepairedWorkstreams, ws.ID)
	}

	return result, nil
}

// extractSessionIDFromContainer attempts to find the current Claude session ID
// by looking at the session files in the container.
func extractSessionIDFromContainer(ctx context.Context, dockerClient *client.Client, containerID string) (string, error) {
	// Look for the most recent session file in the container's Claude projects directory
	// Session files are stored in ~/.claude/projects/-workspace/
	// The filename is the session ID (UUID format)
	cmd := []string{
		"/bin/sh", "-c",
		`ls -t "$HOME/.claude/projects/-workspace/"*.jsonl 2>/dev/null | head -1 | xargs -I{} basename {} .jsonl`,
	}

	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := dockerClient.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec: %w", err)
	}

	attachResp, err := dockerClient.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer attachResp.Close()

	// Read output with timeout
	outputCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		buf := make([]byte, 1024)
		var output strings.Builder
		for {
			n, err := attachResp.Reader.Read(buf)
			if n > 0 {
				output.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		outputCh <- output.String()
	}()

	select {
	case output := <-outputCh:
		// Clean up the output (remove Docker stream headers if present)
		sessionID := cleanDockerOutput(output)
		sessionID = strings.TrimSpace(sessionID)

		// Validate it looks like a UUID or ULID
		if isValidSessionID(sessionID) {
			return sessionID, nil
		}
		return "", nil

	case err := <-errCh:
		return "", err

	case <-time.After(5 * time.Second):
		return "", fmt.Errorf("timeout reading session ID")

	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// cleanDockerOutput removes Docker stream protocol headers from output
func cleanDockerOutput(output string) string {
	// Docker stream protocol prepends 8 bytes of header to each frame
	// We just want the actual text content
	var cleaned strings.Builder
	for _, line := range strings.Split(output, "\n") {
		// Skip empty lines
		if len(line) == 0 {
			continue
		}
		// If line starts with control characters, skip the first 8 bytes
		if len(line) > 8 && line[0] < 32 {
			cleaned.WriteString(line[8:])
		} else {
			cleaned.WriteString(line)
		}
		cleaned.WriteString("\n")
	}
	return cleaned.String()
}

// isValidSessionID checks if a string is a valid UUID or ULID
func isValidSessionID(s string) bool {
	// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (36 chars)
	uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	// ULID format: 26 alphanumeric characters
	ulidRegex := regexp.MustCompile(`^[0-9A-Za-z]{26}$`)

	return uuidRegex.MatchString(s) || ulidRegex.MatchString(s)
}

// ValidateStateOnExit is called when ccells exits to validate and repair state
// Returns a result indicating what was found/repaired, and whether state was saved
func ValidateStateOnExit(ctx context.Context, dir string, workstreams []*Workstream, focusedIndex int, layout int) (*StateRepairResult, error) {
	// First validate and repair
	result, err := ValidateAndRepairState(ctx, workstreams)
	if err != nil {
		return nil, fmt.Errorf("state validation failed: %w", err)
	}

	// If we made repairs, save the updated state
	if result.WasRepaired() {
		if err := SaveState(dir, workstreams, focusedIndex, layout); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to save repaired state: %v", err))
		}
	}

	return result, nil
}
