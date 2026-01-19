package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// ContainerStats holds resource usage statistics for a container.
type ContainerStats struct {
	ContainerID   string
	ContainerName string
	CPUPercent    float64
	MemoryUsage   uint64
	MemoryLimit   uint64
	MemoryPercent float64
	State         string
}

// GetContainerStats retrieves resource usage statistics for a single container.
func (c *Client) GetContainerStats(ctx context.Context, containerID string) (*ContainerStats, error) {
	// Use one-shot stats (stream=false) for a single snapshot
	resp, err := c.cli.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats: %w", err)
	}
	defer resp.Body.Close()

	var stats container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %w", err)
	}

	// Get container info for name and state
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	cpuPercent := calculateCPUPercent(&stats)
	memoryPercent := 0.0
	if stats.MemoryStats.Limit > 0 {
		memoryPercent = float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0
	}

	// Clean container name (remove leading /)
	name := strings.TrimPrefix(info.Name, "/")
	// For ccells containers, strip the ccells- prefix for display
	displayName := strings.TrimPrefix(name, "ccells-")

	return &ContainerStats{
		ContainerID:   containerID,
		ContainerName: displayName,
		CPUPercent:    cpuPercent,
		MemoryUsage:   stats.MemoryStats.Usage,
		MemoryLimit:   stats.MemoryStats.Limit,
		MemoryPercent: memoryPercent,
		State:         info.State.Status,
	}, nil
}

// GetAllCCellsStats retrieves resource usage statistics for all ccells containers.
func (c *Client) GetAllCCellsStats(ctx context.Context) ([]ContainerStats, error) {
	containers, err := c.ListDockerTUIContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var stats []ContainerStats
	for _, cont := range containers {
		// Only get stats for running containers
		if cont.State != "running" {
			// Add entry with zero stats for non-running containers
			name := strings.TrimPrefix(cont.Names[0], "/")
			displayName := strings.TrimPrefix(name, "ccells-")
			stats = append(stats, ContainerStats{
				ContainerID:   cont.ID,
				ContainerName: displayName,
				CPUPercent:    0,
				MemoryUsage:   0,
				MemoryLimit:   0,
				MemoryPercent: 0,
				State:         cont.State,
			})
			continue
		}

		s, err := c.GetContainerStats(ctx, cont.ID)
		if err != nil {
			// Skip containers that fail to get stats
			continue
		}
		stats = append(stats, *s)
	}

	return stats, nil
}

// GetProjectCCellsStats retrieves resource usage statistics for specific container IDs.
func (c *Client) GetProjectCCellsStats(ctx context.Context, containerIDs []string) ([]ContainerStats, error) {
	var stats []ContainerStats
	for _, containerID := range containerIDs {
		if containerID == "" {
			continue
		}

		// Get container state first
		info, err := c.cli.ContainerInspect(ctx, containerID)
		if err != nil {
			// Container may have been removed
			continue
		}

		if info.State.Status != "running" {
			// Add entry with zero stats for non-running containers
			name := strings.TrimPrefix(info.Name, "/")
			displayName := strings.TrimPrefix(name, "ccells-")
			stats = append(stats, ContainerStats{
				ContainerID:   containerID,
				ContainerName: displayName,
				CPUPercent:    0,
				MemoryUsage:   0,
				MemoryLimit:   0,
				MemoryPercent: 0,
				State:         info.State.Status,
			})
			continue
		}

		s, err := c.GetContainerStats(ctx, containerID)
		if err != nil {
			continue
		}
		stats = append(stats, *s)
	}

	return stats, nil
}

// calculateCPUPercent calculates CPU usage percentage from Docker stats.
// The calculation uses the difference between the container's CPU usage and
// the system's CPU usage to determine the percentage.
func calculateCPUPercent(stats *container.StatsResponse) float64 {
	// Get CPU delta values
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)

	if systemDelta <= 0.0 || cpuDelta < 0.0 {
		return 0.0
	}

	// Get number of CPUs
	cpuCount := float64(stats.CPUStats.OnlineCPUs)
	if cpuCount == 0.0 {
		// Fallback to PercpuUsage length if OnlineCPUs is not set
		cpuCount = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
		if cpuCount == 0.0 {
			cpuCount = 1.0
		}
	}

	return (cpuDelta / systemDelta) * cpuCount * 100.0
}

// FormatBytes converts bytes to a human-readable string (KB, MB, GB).
func FormatBytes(bytes uint64) string {
	return formatBytesFloat(float64(bytes))
}

// FormatBytesInt64 converts int64 bytes to a human-readable string (KB, MB, GB).
func FormatBytesInt64(bytes int64) string {
	if bytes < 0 {
		return "0 B"
	}
	return formatBytesFloat(float64(bytes))
}

func formatBytesFloat(bytes float64) string {
	const (
		KB = 1024.0
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", bytes/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", bytes/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", bytes/KB)
	default:
		return fmt.Sprintf("%.0f B", bytes)
	}
}

// DiskUsage holds disk space usage information.
type DiskUsage struct {
	// Container disk usage
	ContainersSize      int64 // Total size of all containers (read-write layers)
	ContainersCount     int   // Number of containers
	CCellsContainerSize int64 // Size of ccells containers specifically

	// Image disk usage
	ImagesSize       int64 // Total size of all images
	ImagesCount      int   // Number of images
	ImagesSharedSize int64 // Shared size between images

	// Volume disk usage
	VolumesSize  int64 // Total size of all volumes
	VolumesCount int   // Number of volumes

	// Build cache
	BuildCacheSize int64 // Total build cache size

	// Total
	TotalSize int64 // Total disk usage
}

// GetDiskUsage retrieves disk space usage information from Docker.
func (c *Client) GetDiskUsage(ctx context.Context) (*DiskUsage, error) {
	du, err := c.cli.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get disk usage: %w", err)
	}

	usage := &DiskUsage{}

	// Container disk usage
	for _, cont := range du.Containers {
		usage.ContainersCount++
		usage.ContainersSize += cont.SizeRw
		// Check if it's a ccells container
		for _, name := range cont.Names {
			if strings.HasPrefix(strings.TrimPrefix(name, "/"), ContainerPrefix) {
				usage.CCellsContainerSize += cont.SizeRw
				break
			}
		}
	}

	// Image disk usage
	for _, img := range du.Images {
		usage.ImagesCount++
		usage.ImagesSize += img.Size
		usage.ImagesSharedSize += img.SharedSize
	}

	// Volume disk usage
	for _, vol := range du.Volumes {
		usage.VolumesCount++
		if vol.UsageData != nil {
			usage.VolumesSize += vol.UsageData.Size
		}
	}

	// Build cache
	if du.BuildCache != nil {
		for _, bc := range du.BuildCache {
			usage.BuildCacheSize += bc.Size
		}
	}

	// Calculate total (use unique image size, not total)
	uniqueImagesSize := usage.ImagesSize - usage.ImagesSharedSize
	if uniqueImagesSize < 0 {
		uniqueImagesSize = usage.ImagesSize
	}
	usage.TotalSize = usage.ContainersSize + uniqueImagesSize + usage.VolumesSize + usage.BuildCacheSize

	return usage, nil
}
