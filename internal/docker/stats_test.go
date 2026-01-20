package docker

import (
	"testing"

	"github.com/docker/docker/api/types"
)

func TestCalculateCPUPercent(t *testing.T) {
	tests := []struct {
		name     string
		stats    *types.Stats
		expected float64
	}{
		{
			name: "zero system delta returns zero",
			stats: &types.Stats{
				CPUStats: types.CPUStats{
					CPUUsage: types.CPUUsage{
						TotalUsage: 1000,
					},
					SystemUsage: 1000,
					OnlineCPUs:  4,
				},
				PreCPUStats: types.CPUStats{
					CPUUsage: types.CPUUsage{
						TotalUsage: 500,
					},
					SystemUsage: 1000, // Same as current - zero delta
				},
			},
			expected: 0.0,
		},
		{
			name: "negative cpu delta returns zero",
			stats: &types.Stats{
				CPUStats: types.CPUStats{
					CPUUsage: types.CPUUsage{
						TotalUsage: 500, // Less than previous
					},
					SystemUsage: 2000,
					OnlineCPUs:  4,
				},
				PreCPUStats: types.CPUStats{
					CPUUsage: types.CPUUsage{
						TotalUsage: 1000,
					},
					SystemUsage: 1000,
				},
			},
			expected: 0.0,
		},
		{
			name: "50% cpu usage with 4 cpus",
			stats: &types.Stats{
				CPUStats: types.CPUStats{
					CPUUsage: types.CPUUsage{
						TotalUsage: 1500,
					},
					SystemUsage: 2000,
					OnlineCPUs:  4,
				},
				PreCPUStats: types.CPUStats{
					CPUUsage: types.CPUUsage{
						TotalUsage: 1000,
					},
					SystemUsage: 1000,
				},
			},
			expected: 200.0, // (500/1000) * 4 * 100 = 200%
		},
		{
			name: "uses percpu length when online cpus is zero",
			stats: &types.Stats{
				CPUStats: types.CPUStats{
					CPUUsage: types.CPUUsage{
						TotalUsage:  1500,
						PercpuUsage: []uint64{1, 2, 3, 4}, // 4 CPUs
					},
					SystemUsage: 2000,
					OnlineCPUs:  0, // Not set
				},
				PreCPUStats: types.CPUStats{
					CPUUsage: types.CPUUsage{
						TotalUsage: 1000,
					},
					SystemUsage: 1000,
				},
			},
			expected: 200.0, // (500/1000) * 4 * 100 = 200%
		},
		{
			name: "defaults to 1 cpu when both are zero",
			stats: &types.Stats{
				CPUStats: types.CPUStats{
					CPUUsage: types.CPUUsage{
						TotalUsage:  1500,
						PercpuUsage: []uint64{}, // Empty
					},
					SystemUsage: 2000,
					OnlineCPUs:  0, // Not set
				},
				PreCPUStats: types.CPUStats{
					CPUUsage: types.CPUUsage{
						TotalUsage: 1000,
					},
					SystemUsage: 1000,
				},
			},
			expected: 50.0, // (500/1000) * 1 * 100 = 50%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateCPUPercent(tt.stats)
			if result != tt.expected {
				t.Errorf("calculateCPUPercent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    uint64
		expected string
	}{
		{
			name:     "bytes",
			bytes:    512,
			expected: "512 B",
		},
		{
			name:     "kilobytes",
			bytes:    1024,
			expected: "1.0 KB",
		},
		{
			name:     "kilobytes with decimal",
			bytes:    1536, // 1.5 KB
			expected: "1.5 KB",
		},
		{
			name:     "megabytes",
			bytes:    1024 * 1024, // 1 MB
			expected: "1.0 MB",
		},
		{
			name:     "megabytes with decimal",
			bytes:    256 * 1024 * 1024, // 256 MB
			expected: "256.0 MB",
		},
		{
			name:     "gigabytes",
			bytes:    1024 * 1024 * 1024, // 1 GB
			expected: "1.0 GB",
		},
		{
			name:     "gigabytes with decimal",
			bytes:    uint64(1.5 * 1024 * 1024 * 1024), // 1.5 GB
			expected: "1.5 GB",
		},
		{
			name:     "zero bytes",
			bytes:    0,
			expected: "0 B",
		},
		{
			name:     "just under 1 KB",
			bytes:    1023,
			expected: "1023 B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestContainerStats(t *testing.T) {
	// Test that ContainerStats struct has all expected fields
	stats := ContainerStats{
		ContainerID:   "abc123",
		ContainerName: "test-container",
		CPUPercent:    25.5,
		MemoryUsage:   256 * 1024 * 1024,  // 256 MB
		MemoryLimit:   1024 * 1024 * 1024, // 1 GB
		MemoryPercent: 25.0,
		State:         "running",
	}

	if stats.ContainerID != "abc123" {
		t.Errorf("ContainerID = %q, want %q", stats.ContainerID, "abc123")
	}
	if stats.ContainerName != "test-container" {
		t.Errorf("ContainerName = %q, want %q", stats.ContainerName, "test-container")
	}
	if stats.CPUPercent != 25.5 {
		t.Errorf("CPUPercent = %v, want %v", stats.CPUPercent, 25.5)
	}
	if stats.MemoryUsage != 256*1024*1024 {
		t.Errorf("MemoryUsage = %v, want %v", stats.MemoryUsage, 256*1024*1024)
	}
	if stats.MemoryLimit != 1024*1024*1024 {
		t.Errorf("MemoryLimit = %v, want %v", stats.MemoryLimit, 1024*1024*1024)
	}
	if stats.MemoryPercent != 25.0 {
		t.Errorf("MemoryPercent = %v, want %v", stats.MemoryPercent, 25.0)
	}
	if stats.State != "running" {
		t.Errorf("State = %q, want %q", stats.State, "running")
	}
}

func TestFormatBytesInt64(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{
			name:     "positive bytes",
			bytes:    512,
			expected: "512 B",
		},
		{
			name:     "positive kilobytes",
			bytes:    1536, // 1.5 KB
			expected: "1.5 KB",
		},
		{
			name:     "positive megabytes",
			bytes:    256 * 1024 * 1024, // 256 MB
			expected: "256.0 MB",
		},
		{
			name:     "positive gigabytes",
			bytes:    int64(2.5 * 1024 * 1024 * 1024), // 2.5 GB
			expected: "2.5 GB",
		},
		{
			name:     "zero bytes",
			bytes:    0,
			expected: "0 B",
		},
		{
			name:     "negative bytes returns zero",
			bytes:    -100,
			expected: "0 B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatBytesInt64(tt.bytes)
			if result != tt.expected {
				t.Errorf("FormatBytesInt64(%d) = %q, want %q", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestDiskUsage(t *testing.T) {
	// Test that DiskUsage struct has all expected fields
	du := DiskUsage{
		ContainersSize:      1024 * 1024 * 100, // 100 MB
		ContainersCount:     5,
		CCellsContainerSize: 1024 * 1024 * 50,   // 50 MB
		ImagesSize:          1024 * 1024 * 1024, // 1 GB
		ImagesCount:         10,
		ImagesSharedSize:    1024 * 1024 * 500, // 500 MB
		VolumesSize:         1024 * 1024 * 200, // 200 MB
		VolumesCount:        3,
		BuildCacheSize:      1024 * 1024 * 50,   // 50 MB
		TotalSize:           1024 * 1024 * 1400, // ~1.4 GB
	}

	if du.ContainersCount != 5 {
		t.Errorf("ContainersCount = %d, want %d", du.ContainersCount, 5)
	}
	if du.ImagesCount != 10 {
		t.Errorf("ImagesCount = %d, want %d", du.ImagesCount, 10)
	}
	if du.VolumesCount != 3 {
		t.Errorf("VolumesCount = %d, want %d", du.VolumesCount, 3)
	}
	if du.CCellsContainerSize != 1024*1024*50 {
		t.Errorf("CCellsContainerSize = %d, want %d", du.CCellsContainerSize, 1024*1024*50)
	}
}
