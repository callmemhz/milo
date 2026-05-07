package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
)

// Logs returns the log stream for the named container.
// Set follow=true to stream live logs; tail controls how many lines to return
// from the end (e.g. "100", "all").
func (c *Client) Logs(ctx context.Context, name string, follow bool, tail string) (io.ReadCloser, error) {
	return c.cli.ContainerLogs(ctx, name, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tail,
		Timestamps: false,
	})
}
