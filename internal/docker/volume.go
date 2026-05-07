package docker

import (
	"context"

	"github.com/docker/docker/api/types/volume"
)

// EnsureVolume creates a named volume if it does not already exist.
// VolumeCreate is idempotent on the same name, so calling it multiple times is safe.
func (c *Client) EnsureVolume(ctx context.Context, name string) error {
	_, err := c.cli.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	return err
}

// RemoveVolume removes the named volume. Set force=true to remove even if in use.
func (c *Client) RemoveVolume(ctx context.Context, name string, force bool) error {
	return c.cli.VolumeRemove(ctx, name, force)
}

// VolumeExists reports whether the named volume exists.
func (c *Client) VolumeExists(ctx context.Context, name string) (bool, error) {
	_, err := c.cli.VolumeInspect(ctx, name)
	if err != nil {
		// Docker SDK wraps the "no such volume" response; treat as not found.
		return false, nil
	}
	return true, nil
}
