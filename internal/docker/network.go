package docker

import (
	"context"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
)

// EnsureNetwork creates the milo bridge network if it does not already exist.
// It is safe to call multiple times (idempotent).
func (c *Client) EnsureNetwork(ctx context.Context) error {
	return c.EnsureNetworkNamed(ctx, c.network, map[string]string{"milo.managed": "true"})
}

// EnsureNetworkNamed creates the named bridge network with the given labels if
// it does not already exist. Idempotent.
func (c *Client) EnsureNetworkNamed(ctx context.Context, name string, labels map[string]string) error {
	args := filters.NewArgs(filters.Arg("name", name))
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{Filters: args})
	if err != nil {
		return err
	}
	for _, n := range networks {
		if n.Name == name {
			return nil
		}
	}

	_, err = c.cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		Labels: labels,
	})
	return err
}

// ConnectNetwork attaches a container to the named network with DNS aliases.
func (c *Client) ConnectNetwork(ctx context.Context, networkName, containerName string, aliases []string) error {
	return c.cli.NetworkConnect(ctx, networkName, containerName, &network.EndpointSettings{
		Aliases: aliases,
	})
}

// RemoveNetwork removes the named network. Exposed for test cleanup.
func (c *Client) RemoveNetwork(ctx context.Context, name string) error {
	return c.cli.NetworkRemove(ctx, name)
}

// ForceRemoveNetwork disconnects all containers from the named network and
// removes it. A missing network is not an error.
func (c *Client) ForceRemoveNetwork(ctx context.Context, name string) error {
	insp, err := c.cli.NetworkInspect(ctx, name, network.InspectOptions{})
	if err != nil {
		if dockerclient.IsErrNotFound(err) {
			return nil
		}
		return err
	}
	for id := range insp.Containers {
		_ = c.cli.NetworkDisconnect(ctx, name, id, true)
	}
	return c.cli.NetworkRemove(ctx, name)
}

// ListNetworksByLabelKey returns name→labels for all networks carrying the
// given label key (any value).
func (c *Client) ListNetworksByLabelKey(ctx context.Context, key string) (map[string]map[string]string, error) {
	args := filters.NewArgs(filters.Arg("label", key))
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{Filters: args})
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]string, len(networks))
	for _, n := range networks {
		out[n.Name] = n.Labels
	}
	return out, nil
}
