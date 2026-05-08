package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/callmemhz/milo-apps-kit/internal/docker"
	"github.com/callmemhz/milo-apps-kit/internal/store"
	"github.com/callmemhz/milo-apps-kit/pkg/api"
)

// Orchestrator manages the full deploy lifecycle for a Milo Apps Kit app.
type Orchestrator struct {
	Store   *store.Store
	Docker  *docker.Client
	LockMgr *LockManager
	Log     *slog.Logger

	// PublishPortForTests is a test-only flag. When true, containers are
	// started with PublishPort=true and health checks use localhost:<hostport>
	// instead of the docker-network IP. Never set this in production.
	PublishPortForTests bool
}

// Locks returns the orchestrator's LockManager, satisfying the server.Deployer interface.
func (o *Orchestrator) Locks() *LockManager { return o.LockMgr }

// DeployRequest carries the parameters for a single deploy.
type DeployRequest struct {
	AppID       int64
	AppName     string
	ImageRef    string
	CommitSHA   string
	GitRef      string
	TriggeredBy int64

	// RegistryAuth is one-shot credentials for pulling ImageRef. It overrides
	// the orchestrator's default Docker client credentials for this deploy
	// only, allowing the caller (CI or operator) to supply short-lived,
	// scope-narrow tokens instead of relying on a long-lived server secret.
	// nil ⇒ fall back to the Docker client's default registry auth.
	RegistryAuth *docker.RegistryAuth
}

// Deploy pulls the image, starts a container, runs a health check, and swaps
// the old container out. It acquires a per-app lock with last-write-wins
// semantics so a concurrent call supersedes this one.
func (o *Orchestrator) Deploy(ctx context.Context, req DeployRequest) (store.Deployment, error) {
	handle, err := o.LockMgr.Acquire(ctx, req.AppName)
	if err != nil {
		return store.Deployment{}, err
	}
	defer handle.Release()
	deployCtx := handle.Context()

	a, err := o.Store.GetAppByID(ctx, req.AppID)
	if err != nil {
		return store.Deployment{}, err
	}

	// Step 1: pull (and resolve digest). RegistryAuth from the caller, if
	// provided, overrides the Docker client's default credentials for this
	// pull only — see DeployRequest.RegistryAuth.
	digest, err := o.Docker.Pull(deployCtx, req.ImageRef, req.RegistryAuth)
	if err != nil {
		return o.recordFailure(ctx, req, "pull_failed", err)
	}

	// Step 2: create deployment row, set status=deploying.
	dep, err := o.Store.CreateDeployment(ctx, req.AppID, digest, req.ImageRef, req.CommitSHA, req.GitRef, req.TriggeredBy)
	if err != nil {
		return store.Deployment{}, err
	}
	if err := o.Store.UpdateDeploymentStatus(ctx, dep.ID, store.DeployDeploying, "", ""); err != nil {
		return store.Deployment{}, err
	}

	containerName := fmt.Sprintf("app-%s-%d", req.AppName, time.Now().UnixNano())
	volumeName := fmt.Sprintf("milo-apps-kit-app-%s-data", req.AppName)
	if err := o.Docker.EnsureVolume(ctx, volumeName); err != nil {
		return o.failExisting(ctx, dep.ID, "docker_error", err)
	}

	env, _ := o.Store.GetAppEnv(ctx, req.AppID)
	if env == nil {
		env = map[string]string{}
	}
	// PaaS contract: the platform tells the app which port to listen on via
	// $PORT. User env can't override this — it's the wire-level invariant
	// that lets Caddy reach the container at a fixed address.
	env["PORT"] = strconv.Itoa(int(a.Port))

	// Use the original image ref to start the container — the image is already
	// pulled locally and Docker resolves it by tag. The digest is stored in DB
	// only for auditability.
	if _, err := o.Docker.Run(deployCtx, docker.RunSpec{
		Name:        containerName,
		Alias:       req.AppName,
		Image:       req.ImageRef,
		Env:         env,
		Port:        int(a.Port),
		CPULimit:    a.CpuLimit,
		MemoryMB:    a.MemoryLimitMb,
		VolumeSrc:   volumeName,
		PublishPort: o.PublishPortForTests,
	}); err != nil {
		return o.failExisting(ctx, dep.ID, "docker_error", err)
	}

	// Step 3: health check.
	// In tests we publish the port and reach localhost:<hostport>.
	// In production we use the container's IP on the docker network.
	var healthHost string
	var healthPort int
	if o.PublishPortForTests {
		info, err := o.Docker.InspectByName(deployCtx, containerName)
		if err != nil {
			_ = o.Docker.Stop(ctx, containerName, 5)
			_ = o.Docker.Remove(ctx, containerName)
			return o.failExisting(ctx, dep.ID, "docker_error", err)
		}
		healthHost = "localhost"
		healthPort = info.HostPort
	} else {
		ip, err := o.Docker.IPInNetwork(deployCtx, containerName)
		if err != nil {
			_ = o.Docker.Stop(ctx, containerName, 5)
			_ = o.Docker.Remove(ctx, containerName)
			return o.failExisting(ctx, dep.ID, "docker_error", err)
		}
		healthHost = ip
		healthPort = int(a.Port)
	}

	if err := o.Docker.HealthCheck(deployCtx, healthHost, healthPort, a.HealthPath, containerName, time.Duration(a.HealthTimeoutSec)*time.Second); err != nil {
		reason := "health_check_failed"
		if deployCtx.Err() != nil {
			reason = "" // superseded — not a failure
		}
		_ = o.Docker.Stop(ctx, containerName, 5)
		_ = o.Docker.Remove(ctx, containerName)
		if reason == "" {
			_ = o.Store.UpdateDeploymentStatus(ctx, dep.ID, store.DeploySuperseded, "", containerName)
			return o.Store.GetDeployment(ctx, dep.ID)
		}
		return o.failExisting(ctx, dep.ID, reason, err)
	}

	// Step 4: swap — stop old container if any.
	if a.CurrentDeployID != nil {
		prev, err := o.Store.GetDeployment(ctx, *a.CurrentDeployID)
		if err == nil && prev.ContainerName != nil && *prev.ContainerName != "" {
			_ = o.Docker.Stop(ctx, *prev.ContainerName, 10)
			_ = o.Docker.Remove(ctx, *prev.ContainerName)
		}
	}

	if err := o.Store.UpdateDeploymentStatus(ctx, dep.ID, store.DeploySucceeded, "", containerName); err != nil {
		return store.Deployment{}, err
	}
	if err := o.Store.SetCurrentDeploy(ctx, req.AppID, dep.ID); err != nil {
		return store.Deployment{}, err
	}
	return o.Store.GetDeployment(ctx, dep.ID)
}

// recordFailure creates a deployment row for a pull failure and marks it failed.
// Returns the deployment record and an api.Error.
func (o *Orchestrator) recordFailure(ctx context.Context, req DeployRequest, reason string, cause error) (store.Deployment, error) {
	dep, _ := o.Store.CreateDeployment(ctx, req.AppID, "", req.ImageRef, req.CommitSHA, req.GitRef, req.TriggeredBy)
	_ = o.Store.UpdateDeploymentStatus(ctx, dep.ID, store.DeployFailed, reason, "")
	out, _ := o.Store.GetDeployment(ctx, dep.ID)
	return out, &api.Error{Code: api.ErrDeployFailed, Message: reason, Details: cause.Error()}
}

// failExisting marks an already-created deployment row as failed.
// Returns both the deployment record and an api.Error.
func (o *Orchestrator) failExisting(ctx context.Context, depID int64, reason string, cause error) (store.Deployment, error) {
	_ = o.Store.UpdateDeploymentStatus(ctx, depID, store.DeployFailed, reason, "")
	out, _ := o.Store.GetDeployment(ctx, depID)
	return out, &api.Error{Code: api.ErrDeployFailed, Message: reason, Details: cause.Error()}
}

// Restart re-deploys the app's current image, acquiring the lock as a new deploy.
func (o *Orchestrator) Restart(ctx context.Context, appID int64) (store.Deployment, error) {
	a, err := o.Store.GetAppByID(ctx, appID)
	if err != nil {
		return store.Deployment{}, err
	}
	if a.CurrentDeployID == nil {
		return store.Deployment{}, &api.Error{Code: api.ErrInvalid, Message: "app has no current deploy"}
	}
	cur, err := o.Store.GetDeployment(ctx, *a.CurrentDeployID)
	if err != nil {
		return store.Deployment{}, err
	}
	return o.Deploy(ctx, DeployRequest{
		AppID:       appID,
		AppName:     a.Name,
		ImageRef:    cur.ImageRef,
		TriggeredBy: cur.TriggeredBy,
	})
}

// DeleteApp stops the running container, optionally removes the data volume,
// and soft-deletes the app record.
func (o *Orchestrator) DeleteApp(ctx context.Context, appID int64, deleteVolume bool) error {
	a, err := o.Store.GetAppByID(ctx, appID)
	if err != nil {
		return err
	}
	handle, _ := o.LockMgr.Acquire(ctx, a.Name)
	defer handle.Release()

	if a.CurrentDeployID != nil {
		cur, err := o.Store.GetDeployment(ctx, *a.CurrentDeployID)
		if err == nil && cur.ContainerName != nil && *cur.ContainerName != "" {
			_ = o.Docker.Stop(ctx, *cur.ContainerName, 5)
			_ = o.Docker.Remove(ctx, *cur.ContainerName)
		}
	}
	if deleteVolume {
		_ = o.Docker.RemoveVolume(ctx, fmt.Sprintf("milo-apps-kit-app-%s-data", a.Name), true)
	}
	return o.Store.SoftDeleteApp(ctx, appID)
}
