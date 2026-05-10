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

// MountSpec is a single bind mount applied to a container.
// Source is a host filesystem path; Target is the in-container path.
type MountSpec struct {
	Source   string
	Target   string
	ReadOnly bool
}

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
	Mounts      []MountSpec
	PublishPort bool
}

// ContainerInfo holds summarised state for a container.
type ContainerInfo struct {
	Name     string
	State    string
	Image    string
	Network  bool
	Labels   map[string]string
	HostPort int
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

	for _, m := range spec.Mounts {
		hc.Mounts = append(hc.Mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}

	// Container config.
	cfg := &container.Config{
		Image:  spec.Image,
		Env:    env,
		Labels: map[string]string{"milo.app": spec.Alias},
	}
	if len(spec.Cmd) > 0 {
		cfg.Cmd = spec.Cmd
	}

	// Port publishing.
	if spec.PublishPort && spec.Port > 0 {
		portKey := nat.Port(fmt.Sprintf("%d/tcp", spec.Port))
		cfg.ExposedPorts = nat.PortSet{portKey: struct{}{}}
		hc.PortBindings = nat.PortMap{
			portKey: []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: "0"},
			},
		}
	}

	// Networking config: attach to milo network with aliases.
	netCfg := &dockernetwork.NetworkingConfig{
		EndpointsConfig: map[string]*dockernetwork.EndpointSettings{
			c.network: {
				Aliases: []string{spec.Alias, spec.Name},
			},
		},
	}

	resp, err := c.cli.ContainerCreate(ctx, cfg, hc, netCfg, nil, spec.Name)
	if err != nil {
		return "", fmt.Errorf("container create: %w", err)
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
		Name:   insp.Name,
		State:  insp.State.Status,
		Image:  insp.Config.Image,
		Labels: insp.Config.Labels,
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
