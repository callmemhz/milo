package docker

import (
	"context"
	"io"
	"strings"

	"github.com/docker/docker/api/types/image"
)

// Pull pulls the image identified by ref and returns its resolved digest
// (sha256:…). Falls back to the image ID if no RepoDigests are available.
func (c *Client) Pull(ctx context.Context, ref string) (string, error) {
	opts := image.PullOptions{}
	if c.pullAuth != "" {
		opts.RegistryAuth = c.pullAuth
	}

	rc, err := c.cli.ImagePull(ctx, ref, opts)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	// Drain the response stream so the pull completes.
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return "", err
	}

	insp, _, err := c.cli.ImageInspectWithRaw(ctx, ref)
	if err != nil {
		return "", err
	}

	for _, d := range insp.RepoDigests {
		// Format: "image@sha256:…"
		if idx := strings.Index(d, "@"); idx >= 0 {
			return d[idx+1:], nil
		}
	}
	// Fallback: use the image content ID.
	return insp.ID, nil
}
