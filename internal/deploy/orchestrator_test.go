//go:build docker_integration

package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/callmemhz/milo-apps-kit/internal/docker"
	"github.com/callmemhz/milo-apps-kit/internal/store"
)

type testSetup struct {
	Store   *store.Store
	Docker  *docker.Client
	Orch    *Orchestrator
	AppID   int64
	AppName string
	TokenID int64
	NetName string
}

func newTestSetup(t *testing.T, appName string) *testSetup {
	t.Helper()
	s, err := store.Open("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	netName := fmt.Sprintf("milo-apps-kit-net-test-%d", time.Now().UnixNano())
	d, err := docker.New(docker.Config{Network: netName})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Ping(context.Background()); err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	if err := d.EnsureNetwork(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = d.RemoveNetwork(context.Background(), netName)
		d.Close()
	})

	ctx := context.Background()
	u, err := s.CreateUser(ctx, "alice", false)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateApp(ctx, appName, 80, "/", 30, 0.5, 128)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddOwner(ctx, a.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	tk, err := s.CreateUserToken(ctx, u.ID, "h", "")
	if err != nil {
		t.Fatal(err)
	}

	orch := &Orchestrator{
		Store:               s,
		Docker:              d,
		LockMgr:             NewLockManager(),
		Log:                 slog.Default(),
		PublishPortForTests: true,
	}

	return &testSetup{
		Store:   s,
		Docker:  d,
		Orch:    orch,
		AppID:   a.ID,
		AppName: appName,
		TokenID: tk.ID,
		NetName: netName,
	}
}

func TestSuccessfulDeploy(t *testing.T) {
	ts := newTestSetup(t, "dtest1")
	ctx := context.Background()

	dep, err := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	})
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	if dep.Status != store.DeploySucceeded {
		t.Fatalf("expected succeeded, got %s (reason: %v)", dep.Status, dep.FailureReason)
	}
	if dep.ContainerName == nil || *dep.ContainerName == "" {
		t.Fatal("expected ContainerName to be set")
	}

	// Verify current_deploy_id is set.
	app, err := ts.Store.GetAppByID(ctx, ts.AppID)
	if err != nil {
		t.Fatal(err)
	}
	if app.CurrentDeployID == nil || *app.CurrentDeployID != dep.ID {
		t.Fatalf("expected CurrentDeployID=%d, got %v", dep.ID, app.CurrentDeployID)
	}

	// Cleanup.
	t.Cleanup(func() {
		_ = ts.Docker.Stop(ctx, *dep.ContainerName, 5)
		_ = ts.Docker.Remove(ctx, *dep.ContainerName)
		_ = ts.Docker.RemoveVolume(ctx, fmt.Sprintf("milo-apps-kit-app-%s-data", ts.AppName), true)
	})

	t.Logf("deployed container: %s", *dep.ContainerName)
}

func TestRolloverReplacesContainer(t *testing.T) {
	ts := newTestSetup(t, "dtest2")
	ctx := context.Background()

	req := DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	}

	dep1, err := ts.Orch.Deploy(ctx, req)
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}
	if dep1.Status != store.DeploySucceeded {
		t.Fatalf("first deploy: expected succeeded, got %s", dep1.Status)
	}
	container1 := *dep1.ContainerName

	dep2, err := ts.Orch.Deploy(ctx, req)
	if err != nil {
		t.Fatalf("second deploy failed: %v", err)
	}
	if dep2.Status != store.DeploySucceeded {
		t.Fatalf("second deploy: expected succeeded, got %s", dep2.Status)
	}
	container2 := *dep2.ContainerName

	if container1 == container2 {
		t.Fatal("expected different container names for v1 and v2")
	}

	// v1 container should be gone.
	_, err = ts.Docker.InspectByName(ctx, container1)
	if err == nil {
		// Cleanup before failing.
		_ = ts.Docker.Stop(ctx, container1, 5)
		_ = ts.Docker.Remove(ctx, container1)
		t.Fatal("v1 container still exists after rollover")
	}

	// v2 container should be running.
	info, err := ts.Docker.InspectByName(ctx, container2)
	if err != nil {
		t.Fatalf("v2 container not found: %v", err)
	}
	if info.State != "running" {
		t.Fatalf("v2 container state: expected running, got %s", info.State)
	}

	t.Cleanup(func() {
		_ = ts.Docker.Stop(ctx, container2, 5)
		_ = ts.Docker.Remove(ctx, container2)
		_ = ts.Docker.RemoveVolume(ctx, fmt.Sprintf("milo-apps-kit-app-%s-data", ts.AppName), true)
	})
}

func TestPullFailureLeavesAppUntouched(t *testing.T) {
	ts := newTestSetup(t, "dtest3")
	ctx := context.Background()

	// First do a successful deploy so there is a current container.
	dep1, err := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	})
	if err != nil {
		t.Fatalf("initial deploy failed: %v", err)
	}
	container1 := *dep1.ContainerName
	t.Cleanup(func() {
		_ = ts.Docker.Stop(ctx, container1, 5)
		_ = ts.Docker.Remove(ctx, container1)
		_ = ts.Docker.RemoveVolume(ctx, fmt.Sprintf("milo-apps-kit-app-%s-data", ts.AppName), true)
	})

	// Now attempt a deploy with a bogus image.
	dep2, err := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nonexistent.invalid/nope:bogus",
		TriggeredBy: ts.TokenID,
	})
	if err == nil {
		t.Fatal("expected error from pull failure, got nil")
	}
	if dep2.Status != store.DeployFailed {
		t.Fatalf("expected failed status, got %s", dep2.Status)
	}
	if dep2.FailureReason == nil || *dep2.FailureReason != "pull_failed" {
		t.Fatalf("expected failure_reason=pull_failed, got %v", dep2.FailureReason)
	}

	// Previous container must still be running.
	info, err := ts.Docker.InspectByName(ctx, container1)
	if err != nil {
		t.Fatalf("original container gone after failed deploy: %v", err)
	}
	if info.State != "running" {
		t.Fatalf("original container state: expected running, got %s", info.State)
	}

	// App's CurrentDeployID must still point to dep1.
	app, err := ts.Store.GetAppByID(ctx, ts.AppID)
	if err != nil {
		t.Fatal(err)
	}
	if app.CurrentDeployID == nil || *app.CurrentDeployID != dep1.ID {
		t.Fatalf("CurrentDeployID changed: expected %d, got %v", dep1.ID, app.CurrentDeployID)
	}
}

func TestHealthCheckFailureLeavesAppUntouched(t *testing.T) {
	ts := newTestSetup(t, "dtest4")
	ctx := context.Background()

	// First successful deploy of nginx on port 80.
	dep1, err := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	})
	if err != nil {
		t.Fatalf("initial deploy failed: %v", err)
	}
	container1 := *dep1.ContainerName
	t.Cleanup(func() {
		_ = ts.Docker.Stop(ctx, container1, 5)
		_ = ts.Docker.Remove(ctx, container1)
		_ = ts.Docker.RemoveVolume(ctx, fmt.Sprintf("milo-apps-kit-app-%s-data", ts.AppName), true)
	})

	// Create a second app configured for port 9999 — nginx won't listen there,
	// so the health check will time out. Use a short timeout.
	a2, err := ts.Store.CreateApp(ctx, "dtest4-bad", 9999, "/", 5, 0.5, 128)
	if err != nil {
		t.Fatal(err)
	}
	// Point the orchestrator at this mis-configured app.
	dep2, err := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       a2.ID,
		AppName:     "dtest4-bad",
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	})

	// Should fail: health check cannot reach port 9999.
	if err == nil {
		if dep2.ContainerName != nil {
			_ = ts.Docker.Stop(ctx, *dep2.ContainerName, 5)
			_ = ts.Docker.Remove(ctx, *dep2.ContainerName)
		}
		t.Fatal("expected health check failure, got nil error")
	}
	if dep2.Status != store.DeployFailed {
		t.Fatalf("expected failed status, got %s", dep2.Status)
	}
	// The bad container must have been cleaned up.
	if dep2.ContainerName != nil && *dep2.ContainerName != "" {
		_, inspectErr := ts.Docker.InspectByName(ctx, *dep2.ContainerName)
		if inspectErr == nil {
			_ = ts.Docker.Stop(ctx, *dep2.ContainerName, 5)
			_ = ts.Docker.Remove(ctx, *dep2.ContainerName)
			t.Fatal("failed container was not cleaned up")
		}
	}

	// Original container (dep1) must still be running.
	info, err := ts.Docker.InspectByName(ctx, container1)
	if err != nil {
		t.Fatalf("original container gone after failed deploy: %v", err)
	}
	if info.State != "running" {
		t.Fatalf("original container state: expected running, got %s", info.State)
	}
}

func TestSupersededByConcurrentDeploy(t *testing.T) {
	ts := newTestSetup(t, "dtest5")
	ctx := context.Background()

	// Start a deploy in the background. We use an image that takes a while
	// (alpine pulling is usually cached, but we rely on the lock being held
	// while pulling). To make the race deterministic, we cancel the first deploy
	// directly via the lock manager after a short delay.
	errCh := make(chan error, 1)
	dep1Ch := make(chan store.Deployment, 1)

	go func() {
		dep, err := ts.Orch.Deploy(ctx, DeployRequest{
			AppID:       ts.AppID,
			AppName:     ts.AppName,
			ImageRef:    "nginx:alpine",
			TriggeredBy: ts.TokenID,
		})
		dep1Ch <- dep
		errCh <- err
	}()

	// Give the goroutine time to acquire the lock and start pulling.
	time.Sleep(100 * time.Millisecond)

	// Cancel the in-flight deploy via the lock manager (simulates a supersede).
	ts.Orch.LockMgr.Cancel(ts.AppName)

	// Wait for the first deploy to finish.
	dep1 := <-dep1Ch
	err1 := <-errCh

	// The first deploy should either be superseded (nil error, superseded status)
	// or a docker_error/pull error if it had already progressed to run. Either
	// way the important invariant is it did not succeed.
	if dep1.Status == store.DeploySucceeded {
		if dep1.ContainerName != nil {
			_ = ts.Docker.Stop(ctx, *dep1.ContainerName, 5)
			_ = ts.Docker.Remove(ctx, *dep1.ContainerName)
		}
		t.Fatal("first deploy should not have succeeded after Cancel")
	}
	_ = err1 // error may or may not be set depending on cancel timing

	t.Logf("first deploy status: %s (err: %v)", dep1.Status, err1)

	// Now do a clean second deploy that should succeed.
	dep2, err2 := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	})
	if err2 != nil {
		t.Fatalf("second deploy failed: %v", err2)
	}
	if dep2.Status != store.DeploySucceeded {
		t.Fatalf("second deploy: expected succeeded, got %s", dep2.Status)
	}

	t.Cleanup(func() {
		if dep2.ContainerName != nil {
			_ = ts.Docker.Stop(ctx, *dep2.ContainerName, 5)
			_ = ts.Docker.Remove(ctx, *dep2.ContainerName)
		}
		_ = ts.Docker.RemoveVolume(ctx, fmt.Sprintf("milo-apps-kit-app-%s-data", ts.AppName), true)
	})

	t.Logf("second deploy container: %s", *dep2.ContainerName)
}

func TestRestart(t *testing.T) {
	ts := newTestSetup(t, "dtest6")
	ctx := context.Background()

	dep1, err := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	})
	if err != nil {
		t.Fatalf("initial deploy failed: %v", err)
	}
	container1 := *dep1.ContainerName

	dep2, err := ts.Orch.Restart(ctx, ts.AppID)
	if err != nil {
		t.Fatalf("restart failed: %v", err)
	}
	if dep2.Status != store.DeploySucceeded {
		t.Fatalf("restart: expected succeeded, got %s", dep2.Status)
	}
	if dep2.ContainerName == nil || *dep2.ContainerName == "" {
		t.Fatal("restart: expected ContainerName to be set")
	}
	container2 := *dep2.ContainerName

	if container1 == container2 {
		t.Fatal("restart should produce a new container name")
	}

	// Old container should be gone.
	_, err = ts.Docker.InspectByName(ctx, container1)
	if err == nil {
		_ = ts.Docker.Stop(ctx, container1, 5)
		_ = ts.Docker.Remove(ctx, container1)
		t.Fatal("old container still exists after restart")
	}

	t.Cleanup(func() {
		_ = ts.Docker.Stop(ctx, container2, 5)
		_ = ts.Docker.Remove(ctx, container2)
		_ = ts.Docker.RemoveVolume(ctx, fmt.Sprintf("milo-apps-kit-app-%s-data", ts.AppName), true)
	})
}

func TestDeleteAppStopsContainer(t *testing.T) {
	ts := newTestSetup(t, "dtest7")
	ctx := context.Background()

	dep, err := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	})
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	containerName := *dep.ContainerName
	volumeName := fmt.Sprintf("milo-apps-kit-app-%s-data", ts.AppName)

	// DeleteApp without removing volume.
	if err := ts.Orch.DeleteApp(ctx, ts.AppID, false); err != nil {
		t.Fatalf("DeleteApp failed: %v", err)
	}

	// Container must be gone.
	_, err = ts.Docker.InspectByName(ctx, containerName)
	if err == nil {
		_ = ts.Docker.Stop(ctx, containerName, 5)
		_ = ts.Docker.Remove(ctx, containerName)
		t.Fatal("container still exists after DeleteApp")
	}

	// Volume must still exist (deleteVolume=false).
	if err := ts.Docker.EnsureVolume(ctx, volumeName); err != nil {
		t.Fatalf("volume not accessible after DeleteApp(false): %v", err)
	}
	t.Cleanup(func() {
		_ = ts.Docker.RemoveVolume(ctx, volumeName, true)
	})

	// App should be soft-deleted — GetAppByID filters deleted_at IS NULL, so
	// it must now return "no rows".
	_, err = ts.Store.GetAppByID(ctx, ts.AppID)
	if err == nil {
		t.Fatal("expected GetAppByID to fail for soft-deleted app")
	}
}

func TestDeleteAppRemovesVolume(t *testing.T) {
	ts := newTestSetup(t, "dtest8")
	ctx := context.Background()

	dep, err := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	})
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	_ = dep
	volumeName := fmt.Sprintf("milo-apps-kit-app-%s-data", ts.AppName)

	if err := ts.Orch.DeleteApp(ctx, ts.AppID, true); err != nil {
		t.Fatalf("DeleteApp(volumes=true) failed: %v", err)
	}

	// Volume must be gone.
	exists, err := ts.Docker.VolumeExists(ctx, volumeName)
	if err != nil {
		t.Fatalf("VolumeExists check failed: %v", err)
	}
	if exists {
		_ = ts.Docker.RemoveVolume(ctx, volumeName, true)
		t.Fatal("volume still exists after DeleteApp(volumes=true)")
	}
}
