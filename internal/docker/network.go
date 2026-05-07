package docker

import (
	"context"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
)

// EnsureNetwork creates the milo-apps-kit bridge network if it does not already exist.
// It is safe to call multiple times (idempotent).
func (c *Client) EnsureNetwork(ctx context.Context) error {
	args := filters.NewArgs(filters.Arg("name", c.network))
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{Filters: args})
	if err != nil {
		return err
	}
	for _, n := range networks {
		if n.Name == c.network {
			return nil
		}
	}

	_, err = c.cli.NetworkCreate(ctx, c.network, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{"milo-apps-kit.managed": "true"},
	})
	return err
}

// RemoveNetwork removes the named network. Exposed for test cleanup.
func (c *Client) RemoveNetwork(ctx context.Context, name string) error {
	return c.cli.NetworkRemove(ctx, name)
}
