package docker

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
)

// DiskUsage summarises docker-managed disk consumption plus per-volume sizes.
type DiskUsage struct {
	ImagesSize     int64
	ContainersSize int64
	VolumesSize    int64
	BuildCacheSize int64
	Volumes        map[string]int64 // volume name -> bytes
}

func (c *Client) DiskUsage(ctx context.Context) (DiskUsage, error) {
	du, err := c.cli.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		return DiskUsage{}, err
	}
	out := DiskUsage{ImagesSize: du.LayersSize, Volumes: map[string]int64{}}
	for _, ct := range du.Containers {
		out.ContainersSize += ct.SizeRw
	}
	for _, v := range du.Volumes {
		if v.UsageData != nil && v.UsageData.Size > 0 {
			out.VolumesSize += v.UsageData.Size
			out.Volumes[v.Name] = v.UsageData.Size
		} else {
			out.Volumes[v.Name] = -1 // size not computed
		}
	}
	for _, bc := range du.BuildCache {
		out.BuildCacheSize += bc.Size
	}
	return out, nil
}

// VolumeSize returns a single volume's size in bytes; ok=false if unknown.
func (c *Client) VolumeSize(ctx context.Context, name string) (int64, bool) {
	du, err := c.DiskUsage(ctx)
	if err != nil {
		return 0, false
	}
	sz, present := du.Volumes[name]
	if !present || sz < 0 {
		return 0, false
	}
	return sz, true
}

// Image is a summarised local image.
type Image struct {
	ID       string
	RepoTags []string
	Size     int64
	Created  int64 // unix seconds
}

func (c *Client) ImageList(ctx context.Context) ([]Image, error) {
	sums, err := c.cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Image, 0, len(sums))
	for _, s := range sums {
		out = append(out, Image{ID: s.ID, RepoTags: s.RepoTags, Size: s.Size, Created: s.Created})
	}
	return out, nil
}

func (c *Client) ImageRemove(ctx context.Context, id string, force bool) error {
	_, err := c.cli.ImageRemove(ctx, id, image.RemoveOptions{Force: force, PruneChildren: true})
	return err
}

// HostLoad is host-level load read from /proc (kernel-global, so visible from
// inside the milod container — reflects the docker host/VM, not the container).
type HostLoad struct {
	Load1, Load5, Load15 float64
	MemTotal, MemAvail   uint64 // bytes
	OK                   bool   // false when /proc is unavailable (e.g. macOS)
}

func ReadHostLoad() HostLoad {
	var h HostLoad
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		f := strings.Fields(string(b))
		if len(f) >= 3 {
			h.Load1, _ = strconv.ParseFloat(f[0], 64)
			h.Load5, _ = strconv.ParseFloat(f[1], 64)
			h.Load15, _ = strconv.ParseFloat(f[2], 64)
			h.OK = true
		}
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			kb, _ := strconv.ParseUint(fields[1], 10, 64)
			switch fields[0] {
			case "MemTotal:":
				h.MemTotal = kb * 1024
			case "MemAvailable:":
				h.MemAvail = kb * 1024
			}
		}
	}
	return h
}
