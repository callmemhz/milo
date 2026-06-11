package docker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
)

// execOnce runs cmd inside the named container and returns its exit code.
func (c *Client) execOnce(ctx context.Context, containerName string, cmd []string) (int, error) {
	resp, err := c.cli.ContainerExecCreate(ctx, containerName, container.ExecOptions{Cmd: cmd})
	if err != nil {
		return -1, err
	}
	if err := c.cli.ContainerExecStart(ctx, resp.ID, container.ExecStartOptions{}); err != nil {
		return -1, err
	}
	for {
		insp, err := c.cli.ContainerExecInspect(ctx, resp.ID)
		if err != nil {
			return -1, err
		}
		if !insp.Running {
			return insp.ExitCode, nil
		}
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// ExecProbe polls cmd inside the container until it exits 0 or the timeout
// elapses. It is the readiness check for non-HTTP workloads (e.g. pg_isready,
// redis-cli ping). Fails fast if the container exits.
func (c *Client) ExecProbe(ctx context.Context, containerName string, cmd []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		code, err := c.execOnce(ctx, containerName, cmd)
		if err == nil && code == 0 {
			return nil
		}
		lastErr = err
		info, ierr := c.InspectByName(ctx, containerName)
		if ierr == nil && info.State == "exited" {
			return errors.New("container exited during readiness probe")
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		return fmt.Errorf("readiness probe timeout: %w", lastErr)
	}
	return errors.New("readiness probe timeout")
}
