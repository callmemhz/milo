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

// newHygieneSetup creates an isolated test environment for hygiene tests.
// It uses its own network to avoid interference with other tests.
func newHygieneSetup(t *testing.T, appName string) *testSetup {
	t.Helper()
	s, err := store.Open("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	netName := fmt.Sprintf("milo-apps-kit-net-hygiene-%d", time.Now().UnixNano())
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
	var appID int64
	var tokenID int64
	if appName != "" {
		a, err := s.CreateApp(ctx, appName, 80, "/", 30, 0.5, 128)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.AddOwner(ctx, a.ID, u.ID); err != nil {
			t.Fatal(err)
		}
		appID = a.ID
	}
	tk, err := s.CreateUserToken(ctx, u.ID, "h", "")
	if err != nil {
		t.Fatal(err)
	}
	tokenID = tk.ID

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
		AppID:   appID,
		AppName: appName,
		TokenID: tokenID,
		NetName: netName,
	}
}

func newHygiene(ts *testSetup) *Hygiene {
	return &Hygiene{
		Store:  ts.Store,
		Docker: ts.Docker,
		Log:    slog.Default(),
	}
}

// TestHygieneMarksInflightFailed inserts a deploying deployment row, runs
// hygiene, and verifies the status becomes failed.
func TestHygieneMarksInflightFailed(t *testing.T) {
	ts := newHygieneSetup(t, "htest1")
	ctx := context.Background()

	dep, err := ts.Store.CreateDeployment(ctx, ts.AppID, "sha256:abc", "nginx:alpine", "", "", ts.TokenID)
	if err != nil {
		t.Fatal(err)
	}
	// Mark as deploying to simulate a crash mid-flight.
	if err := ts.Store.UpdateDeploymentStatus(ctx, dep.ID, store.DeployDeploying, "", ""); err != nil {
		t.Fatal(err)
	}

	h := newHygiene(ts)
	if err := h.Run(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := ts.Store.GetDeployment(ctx, dep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.DeployFailed {
		t.Fatalf("expected failed, got %s", got.Status)
	}
	if got.FailureReason == nil || *got.FailureReason != "docker_error" {
		t.Fatalf("expected failure_reason=docker_error, got %v", got.FailureReason)
	}
}

// TestHygieneRemovesOrphanWhenAppMissing starts a container labelled with an
// app name that has no matching DB row. Hygiene must remove it.
func TestHygieneRemovesOrphanWhenAppMissing(t *testing.T) {
	ts := newHygieneSetup(t, "") // no app created in DB
	ctx := context.Background()

	cName := fmt.Sprintf("orphan-test-%d", time.Now().UnixNano())
	_, err := ts.Docker.Run(ctx, docker.RunSpec{
		Name:     cName,
		Alias:    "nope",
		Image:    "nginx:alpine",
		Port:     80,
		MemoryMB: 64,
	})
	if err != nil {
		t.Fatalf("failed to start orphan container: %v", err)
	}
	t.Cleanup(func() {
		// best-effort; hygiene should remove it
		_ = ts.Docker.Stop(ctx, cName, 5)
		_ = ts.Docker.Remove(ctx, cName)
	})

	h := newHygiene(ts)
	if err := h.Run(ctx); err != nil {
		t.Fatal(err)
	}

	_, err = ts.Docker.InspectByName(ctx, cName)
	if err == nil {
		t.Fatal("orphan container still exists after hygiene")
	}
}

// TestHygieneRemovesStaleRevision deploys v1, then manually starts a second
// container with the same app label (simulating a stale leftover). Hygiene must
// remove the stale container and keep v1.
func TestHygieneRemovesStaleRevision(t *testing.T) {
	ts := newHygieneSetup(t, "htest3")
	ctx := context.Background()

	// Deploy v1 via orchestrator.
	dep1, err := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	})
	if err != nil {
		t.Fatalf("v1 deploy failed: %v", err)
	}
	if dep1.Status != store.DeploySucceeded {
		t.Fatalf("v1: expected succeeded, got %s", dep1.Status)
	}
	container1 := *dep1.ContainerName

	t.Cleanup(func() {
		_ = ts.Docker.Stop(ctx, container1, 5)
		_ = ts.Docker.Remove(ctx, container1)
		_ = ts.Docker.RemoveVolume(ctx, fmt.Sprintf("milo-apps-kit-app-%s-data", ts.AppName), true)
	})

	// Start a stale container with the same app label but a different name.
	staleName := fmt.Sprintf("stale-%s-%d", ts.AppName, time.Now().UnixNano())
	_, err = ts.Docker.Run(ctx, docker.RunSpec{
		Name:     staleName,
		Alias:    ts.AppName, // same label
		Image:    "nginx:alpine",
		Port:     80,
		MemoryMB: 64,
	})
	if err != nil {
		t.Fatalf("failed to start stale container: %v", err)
	}
	t.Cleanup(func() {
		_ = ts.Docker.Stop(ctx, staleName, 5)
		_ = ts.Docker.Remove(ctx, staleName)
	})

	h := newHygiene(ts)
	if err := h.Run(ctx); err != nil {
		t.Fatal(err)
	}

	// Stale container must be gone.
	_, err = ts.Docker.InspectByName(ctx, staleName)
	if err == nil {
		t.Fatal("stale container still exists after hygiene")
	}

	// v1 container must still be running.
	info, err := ts.Docker.InspectByName(ctx, container1)
	if err != nil {
		t.Fatalf("v1 container removed by hygiene: %v", err)
	}
	if info.State != "running" {
		t.Fatalf("v1 container not running after hygiene: %s", info.State)
	}
}

// TestHygieneKeepsCurrentContainer deploys v1 and verifies hygiene leaves it alone.
func TestHygieneKeepsCurrentContainer(t *testing.T) {
	ts := newHygieneSetup(t, "htest4")
	ctx := context.Background()

	dep1, err := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	})
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if dep1.Status != store.DeploySucceeded {
		t.Fatalf("expected succeeded, got %s", dep1.Status)
	}
	container1 := *dep1.ContainerName

	t.Cleanup(func() {
		_ = ts.Docker.Stop(ctx, container1, 5)
		_ = ts.Docker.Remove(ctx, container1)
		_ = ts.Docker.RemoveVolume(ctx, fmt.Sprintf("milo-apps-kit-app-%s-data", ts.AppName), true)
	})

	h := newHygiene(ts)
	if err := h.Run(ctx); err != nil {
		t.Fatal(err)
	}

	info, err := ts.Docker.InspectByName(ctx, container1)
	if err != nil {
		t.Fatalf("current container removed by hygiene: %v", err)
	}
	if info.State != "running" {
		t.Fatalf("current container not running after hygiene: %s", info.State)
	}
}
