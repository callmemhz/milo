package deploy

import (
	"context"
	"log/slog"
	"strings"

	"github.com/callmemhz/milo-apps-kit/internal/docker"
	"github.com/callmemhz/milo-apps-kit/internal/store"
)

// Hygiene cleans up orphaned/stale containers and marks crashed in-flight
// deployments as failed. Intended to run once at server startup.
type Hygiene struct {
	Store  *store.Store
	Docker *docker.Client
	Log    *slog.Logger
}

// Run does three things at startup:
// 1. Mark any deployment row in pending/deploying as failed (we crashed mid-flight)
// 2. List milo-apps-kit-net containers; for each:
//   - if no matching app (or app soft-deleted) → orphan, remove
//   - if app exists but container_name doesn't match the current deploy's container_name → stale, remove
//   - else keep
func (h *Hygiene) Run(ctx context.Context) error {
	inflight, err := h.Store.ListInflightDeployments(ctx)
	if err != nil {
		return err
	}
	for _, d := range inflight {
		cn := ""
		if d.ContainerName != nil {
			cn = *d.ContainerName
		}
		_ = h.Store.UpdateDeploymentStatus(ctx, d.ID, store.DeployFailed, "docker_error", cn)
	}

	containers, err := h.Docker.ListOnNetwork(ctx)
	if err != nil {
		return err
	}
	for _, c := range containers {
		appName := c.Labels["milo-apps-kit.app"]
		if appName == "" {
			continue
		}
		primary := containerPrimaryName(c.Names)

		a, err := h.Store.GetAppByName(ctx, appName)
		if err != nil {
			// app gone → orphan
			h.Log.Info("removing orphan", "container", primary, "app", appName)
			_ = h.Docker.Remove(ctx, primary)
			continue
		}
		if a.CurrentDeployID == nil {
			// app exists but never deployed (or current cleared) → stale
			h.Log.Info("removing stale (no current deploy)", "container", primary, "app", appName)
			_ = h.Docker.Remove(ctx, primary)
			continue
		}
		cur, err := h.Store.GetDeployment(ctx, *a.CurrentDeployID)
		if err != nil || cur.ContainerName == nil || *cur.ContainerName != primary {
			h.Log.Info("removing stale revision", "container", primary, "app", appName)
			_ = h.Docker.Remove(ctx, primary)
			continue
		}
		// current — keep
	}
	return nil
}

func containerPrimaryName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}
