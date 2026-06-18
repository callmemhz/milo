package docker

import (
	"context"
	"fmt"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

// RunSpec describes how to create and start a container.
type RunSpec struct {
	Name        string
	Alias       string
	Image       string
	Env         map[string]string
	Cmd         []string
	Port        int
	CPULimit    float64
	MemoryMB    int64
	VolumeSrc   string
	PublishPort bool
	// PublishHostPort pins the host port that Port is published on. 0 means
	// let Docker pick an ephemeral port. Only consulted when PublishPort is set.
	PublishHostPort int

	// Network overrides the primary network the container joins
	// (default: the client's milo network).
	Network string
	// ExtraNetworks are additional networks to attach before start, with the
	// same aliases as the primary network. Used to give app containers access
	// to the per-addon networks of their linked add-ons.
	ExtraNetworks []string
	// VolumeTarget is the mount point for VolumeSrc (default: /data).
	VolumeTarget string
	// Labels overrides the container labels (default: {"milo.app": Alias}).
	Labels map[string]string
}

// ContainerInfo holds summarised state for a container.
type ContainerInfo struct {
	Name         string
	State        string
	Image        string
	Network      bool
	Labels       map[string]string
	HostPort     int
	StartedAt    string // RFC3339, empty if never started
	RestartCount int
}

// Run creates and starts a container according to spec. Returns the container ID.
func (c *Client) Run(ctx context.Context, spec RunSpec) (string, error) {
	// Build env slice.
	var env []string
	for k, v := range spec.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Build HostConfig.
	hc := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
		Resources: container.Resources{
			NanoCPUs: int64(spec.CPULimit * 1e9),
			Memory:   spec.MemoryMB * 1024 * 1024,
		},
	}

	if spec.VolumeSrc != "" {
		target := spec.VolumeTarget
		if target == "" {
			target = "/data"
		}
		hc.Mounts = []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: spec.VolumeSrc,
				Target: target,
			},
		}
	}

	labels := spec.Labels
	if labels == nil {
		labels = map[string]string{"milo.app": spec.Alias}
	}

	// Container config.
	cfg := &container.Config{
		Image:  spec.Image,
		Env:    env,
		Labels: labels,
	}
	if len(spec.Cmd) > 0 {
		cfg.Cmd = spec.Cmd
	}

	// Port publishing.
	if spec.PublishPort && spec.Port > 0 {
		hostPort := "0"
		if spec.PublishHostPort > 0 {
			hostPort = strconv.Itoa(spec.PublishHostPort)
		}
		portKey := nat.Port(fmt.Sprintf("%d/tcp", spec.Port))
		cfg.ExposedPorts = nat.PortSet{portKey: struct{}{}}
		hc.PortBindings = nat.PortMap{
			portKey: []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: hostPort},
			},
		}
	}

	// Networking config: attach to the primary network with aliases.
	primaryNet := spec.Network
	if primaryNet == "" {
		primaryNet = c.network
	}
	netCfg := &dockernetwork.NetworkingConfig{
		EndpointsConfig: map[string]*dockernetwork.EndpointSettings{
			primaryNet: {
				Aliases: []string{spec.Alias, spec.Name},
			},
		},
	}

	resp, err := c.cli.ContainerCreate(ctx, cfg, hc, netCfg, nil, spec.Name)
	if err != nil {
		return "", fmt.Errorf("container create: %w", err)
	}

	// Attach extra networks before start so DNS is in place from boot.
	for _, n := range spec.ExtraNetworks {
		if err := c.ConnectNetwork(ctx, n, resp.ID, []string{spec.Alias, spec.Name}); err != nil {
			_ = c.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
			return "", fmt.Errorf("network connect %s: %w", n, err)
		}
	}

	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Force-remove on start failure.
		_ = c.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("container start: %w", err)
	}

	return resp.ID, nil
}

// Stop stops the named container, waiting up to timeoutSec seconds.
func (c *Client) Stop(ctx context.Context, name string, timeoutSec int) error {
	t := timeoutSec
	return c.cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &t})
}

// Remove force-removes the named container.
func (c *Client) Remove(ctx context.Context, name string) error {
	return c.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
}

// InspectByName returns ContainerInfo for the named container.
func (c *Client) InspectByName(ctx context.Context, name string) (*ContainerInfo, error) {
	insp, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return nil, err
	}

	info := &ContainerInfo{
		Name:         insp.Name,
		State:        insp.State.Status,
		Image:        insp.Config.Image,
		Labels:       insp.Config.Labels,
		RestartCount: insp.RestartCount,
	}
	if insp.State != nil {
		info.StartedAt = insp.State.StartedAt
	}

	// Check if container is on the milo network.
	if insp.NetworkSettings != nil {
		_, info.Network = insp.NetworkSettings.Networks[c.network]

		// Resolve host port: scan all port bindings for any tcp mapping.
		for _, bindings := range insp.NetworkSettings.Ports {
			if len(bindings) > 0 {
				if hp, err := strconv.Atoi(bindings[0].HostPort); err == nil && hp > 0 {
					info.HostPort = hp
					break
				}
			}
		}
	}

	return info, nil
}

// ListOnNetwork returns all containers (running or stopped) on the milo network.
func (c *Client) ListOnNetwork(ctx context.Context) ([]container.Summary, error) {
	args := filters.NewArgs(filters.Arg("network", c.network))
	return c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
}

// ListByLabelKey returns all containers (running or stopped) carrying the
// given label key, regardless of which network they are on.
func (c *Client) ListByLabelKey(ctx context.Context, key string) ([]container.Summary, error) {
	args := filters.NewArgs(filters.Arg("label", key))
	return c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
}

// IPInNetwork returns the container's IP address on the milo network.
func (c *Client) IPInNetwork(ctx context.Context, name string) (string, error) {
	insp, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		return "", err
	}
	if insp.NetworkSettings == nil {
		return "", fmt.Errorf("container %s has no network settings", name)
	}
	ep, ok := insp.NetworkSettings.Networks[c.network]
	if !ok {
		return "", fmt.Errorf("container %s is not on network %s", name, c.network)
	}
	return ep.IPAddress, nil
}
