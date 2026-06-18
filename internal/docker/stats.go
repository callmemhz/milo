package docker

import (
	"context"
	"encoding/json"
	"io"

	"github.com/docker/docker/api/types/container"
)

// Stats is a single resource-usage sample for a container.
type Stats struct {
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryUsage uint64  `json:"memory_usage"` // bytes
	MemoryLimit uint64  `json:"memory_limit"` // bytes
}

// StatsStream opens the Docker streaming stats endpoint for a container. Each
// frame decoded from the returned reader carries precpu_stats from the previous
// frame, which is required to compute a CPU percentage. Caller must Close it.
func (c *Client) StatsStream(ctx context.Context, name string) (io.ReadCloser, error) {
	resp, err := c.cli.ContainerStats(ctx, name, true)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// SampleStats reads two frames from the stats stream (~1s apart) and returns a
// single sample with a valid CPU percentage. Use this for one-shot reads; for
// live updates consume StatsStream directly via ParseStats.
func (c *Client) SampleStats(ctx context.Context, name string) (Stats, error) {
	rc, err := c.StatsStream(ctx, name)
	if err != nil {
		return Stats{}, err
	}
	defer rc.Close()

	dec := json.NewDecoder(rc)
	var last container.StatsResponse
	for i := 0; i < 2; i++ {
		if err := dec.Decode(&last); err != nil {
			if i == 0 {
				return Stats{}, err
			}
			break
		}
	}
	return statsFromResponse(last), nil
}

// ParseStats converts a decoded Docker stats frame into a Stats sample.
func ParseStats(v container.StatsResponse) Stats { return statsFromResponse(v) }

func statsFromResponse(v container.StatsResponse) Stats {
	s := Stats{
		MemoryUsage: v.MemoryStats.Usage,
		MemoryLimit: v.MemoryStats.Limit,
	}
	// Subtract cache page count the way `docker stats` does, when available.
	if cache, ok := v.MemoryStats.Stats["inactive_file"]; ok && cache <= s.MemoryUsage {
		s.MemoryUsage -= cache
	}

	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
	if cpuDelta > 0 && sysDelta > 0 {
		ncpu := float64(v.CPUStats.OnlineCPUs)
		if ncpu == 0 {
			ncpu = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
		}
		if ncpu == 0 {
			ncpu = 1
		}
		s.CPUPercent = (cpuDelta / sysDelta) * ncpu * 100.0
	}
	return s
}
