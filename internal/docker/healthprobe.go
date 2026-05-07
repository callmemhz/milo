package docker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// HealthCheck polls http://host:port/path until 2xx or timeout. Fails fast if
// the named container exits. Pass containerName="" to skip exit detection.
func (c *Client) HealthCheck(ctx context.Context, host string, port int, path, containerName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://%s:%d%s", host, port, path)
	httpClient := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := httpClient.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		if containerName != "" {
			info, ierr := c.InspectByName(ctx, containerName)
			if ierr == nil && info.State == "exited" {
				return errors.New("container exited during health check")
			}
		}
		time.Sleep(time.Second)
	}
	return errors.New("health check timeout")
}
