//go:build docker_integration

package deploy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/callmemhz/milo/internal/store"
)

// cleanupAddon removes everything a provisioned addon leaves behind.
func cleanupAddon(t *testing.T, ts *testSetup, name string) {
	t.Helper()
	ctx := context.Background()
	_ = ts.Docker.Stop(ctx, AddonContainerName(name), 5)
	_ = ts.Docker.Remove(ctx, AddonContainerName(name))
	_ = ts.Docker.ForceRemoveNetwork(ctx, AddonNetworkName(name))
	_ = ts.Docker.RemoveVolume(ctx, AddonVolumeName(name), true)
}

func TestProvisionAndDeleteRedisAddon(t *testing.T) {
	ts := newTestSetup(t, "adapp1")
	ctx := context.Background()

	name := fmt.Sprintf("itr%d", time.Now().UnixNano()%1e9)
	t.Cleanup(func() { cleanupAddon(t, ts, name) })

	pass, err := GeneratePassword()
	if err != nil {
		t.Fatal(err)
	}
	addon, err := ts.Store.CreateAddon(ctx, name, "redis", "7", 0.5, 256, pass)
	if err != nil {
		t.Fatal(err)
	}

	if err := ts.Orch.ProvisionAddon(ctx, addon.ID); err != nil {
		t.Fatalf("provision failed: %v", err)
	}
	got, _ := ts.Store.GetAddonByID(ctx, addon.ID)
	if got.Status != store.AddonRunning {
		t.Fatalf("status: %s", got.Status)
	}
	info, err := ts.Docker.InspectByName(ctx, AddonContainerName(name))
	if err != nil || info.State != "running" {
		t.Fatalf("container: %v %+v", err, info)
	}

	// Restart path: re-provision recreates in place and comes back running.
	if err := ts.Orch.ProvisionAddon(ctx, addon.ID); err != nil {
		t.Fatalf("re-provision failed: %v", err)
	}
	got, _ = ts.Store.GetAddonByID(ctx, addon.ID)
	if got.Status != store.AddonRunning {
		t.Fatalf("status after restart: %s", got.Status)
	}

	// Delete tears down container, network, and (with flag) the volume.
	if err := ts.Orch.DeleteAddon(ctx, addon.ID, true); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := ts.Docker.InspectByName(ctx, AddonContainerName(name)); err == nil {
		t.Fatal("container still exists after delete")
	}
	exists, _ := ts.Docker.VolumeExists(ctx, AddonVolumeName(name))
	if exists {
		t.Fatal("volume still exists after delete")
	}
	nets, _ := ts.Docker.ListNetworksByLabelKey(ctx, "milo.addon")
	if _, ok := nets[AddonNetworkName(name)]; ok {
		t.Fatal("network still exists after delete")
	}
	if _, err := ts.Store.GetAddonByID(ctx, addon.ID); err == nil {
		t.Fatal("expected soft-deleted addon to be hidden")
	}
}

func TestExposeAddonPublishesStableHostPort(t *testing.T) {
	ts := newTestSetup(t, "adapp3")
	ctx := context.Background()

	name := fmt.Sprintf("itx%d", time.Now().UnixNano()%1e9)
	t.Cleanup(func() { cleanupAddon(t, ts, name) })

	pass, _ := GeneratePassword()
	addon, err := ts.Store.CreateAddon(ctx, name, "redis", "7", 0.5, 256, pass)
	if err != nil {
		t.Fatal(err)
	}

	// Exposing: flip the flag, provision, and the host port is assigned + persisted.
	if err := ts.Store.SetAddonExposed(ctx, addon.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := ts.Orch.ProvisionAddon(ctx, addon.ID); err != nil {
		t.Fatalf("provision (exposed) failed: %v", err)
	}
	got, _ := ts.Store.GetAddonByID(ctx, addon.ID)
	if got.HostPort == 0 {
		t.Fatal("expected a host port to be assigned on expose")
	}
	info, err := ts.Docker.InspectByName(ctx, AddonContainerName(name))
	if err != nil || info.HostPort != int(got.HostPort) {
		t.Fatalf("published host port mismatch: db=%d docker=%+v err=%v", got.HostPort, info, err)
	}

	// Restart keeps the same host port (stable connection string).
	first := got.HostPort
	if err := ts.Orch.ProvisionAddon(ctx, addon.ID); err != nil {
		t.Fatalf("re-provision failed: %v", err)
	}
	got, _ = ts.Store.GetAddonByID(ctx, addon.ID)
	if got.HostPort != first {
		t.Fatalf("host port changed across restart: %d -> %d", first, got.HostPort)
	}
	info, _ = ts.Docker.InspectByName(ctx, AddonContainerName(name))
	if info.HostPort != int(first) {
		t.Fatalf("docker host port changed across restart: want %d got %d", first, info.HostPort)
	}

	// Unexpose: provision without the published port; host_port is retained.
	if err := ts.Store.SetAddonExposed(ctx, addon.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := ts.Orch.ProvisionAddon(ctx, addon.ID); err != nil {
		t.Fatalf("provision (unexposed) failed: %v", err)
	}
	info, _ = ts.Docker.InspectByName(ctx, AddonContainerName(name))
	if info.HostPort != 0 {
		t.Fatalf("expected no published port after unexpose, got %d", info.HostPort)
	}
	got, _ = ts.Store.GetAddonByID(ctx, addon.ID)
	if got.HostPort != first {
		t.Fatalf("host port should be retained after unexpose: want %d got %d", first, got.HostPort)
	}
}

func TestProvisionPostgresAddon(t *testing.T) {
	ts := newTestSetup(t, "adapp2")
	ctx := context.Background()

	name := fmt.Sprintf("itp%d", time.Now().UnixNano()%1e9)
	t.Cleanup(func() { cleanupAddon(t, ts, name) })

	pass, _ := GeneratePassword()
	addon, err := ts.Store.CreateAddon(ctx, name, "postgres", "16", 0.5, 512, pass)
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.Orch.ProvisionAddon(ctx, addon.ID); err != nil {
		t.Fatalf("provision failed: %v", err)
	}
	got, _ := ts.Store.GetAddonByID(ctx, addon.ID)
	if got.Status != store.AddonRunning {
		t.Fatalf("status: %s", got.Status)
	}
	// pg_isready already gated readiness; double-check we can run a query.
	if err := ts.Docker.ExecProbe(ctx, AddonContainerName(name),
		[]string{"psql", "-U", "app", "-d", "app", "-c", "SELECT 1"}, 15*time.Second); err != nil {
		t.Fatalf("psql: %v", err)
	}
	if err := ts.Orch.DeleteAddon(ctx, addon.ID, true); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}

func TestDeployLinkedAppGetsEnvAndNetwork(t *testing.T) {
	ts := newTestSetup(t, "adapp3")
	ctx := context.Background()

	name := fmt.Sprintf("itl%d", time.Now().UnixNano()%1e9)
	t.Cleanup(func() { cleanupAddon(t, ts, name) })

	pass, _ := GeneratePassword()
	addon, err := ts.Store.CreateAddon(ctx, name, "redis", "7", 0.5, 256, pass)
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.Orch.ProvisionAddon(ctx, addon.ID); err != nil {
		t.Fatalf("provision failed: %v", err)
	}
	if _, err := ts.Store.CreateLink(ctx, ts.AppID, addon.ID, ""); err != nil {
		t.Fatal(err)
	}

	dep, err := ts.Orch.Deploy(ctx, DeployRequest{
		AppID:       ts.AppID,
		AppName:     ts.AppName,
		ImageRef:    "nginx:alpine",
		TriggeredBy: ts.TokenID,
	})
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	t.Cleanup(func() {
		_ = ts.Docker.Stop(ctx, *dep.ContainerName, 5)
		_ = ts.Docker.Remove(ctx, *dep.ContainerName)
		_ = ts.Docker.RemoveVolume(ctx, fmt.Sprintf("milo-app-%s-data", ts.AppName), true)
	})

	// REDIS_URL must be injected into the app container.
	want := ConnectionURL("redis", name, pass)
	if err := ts.Docker.ExecProbe(ctx, *dep.ContainerName,
		[]string{"sh", "-c", fmt.Sprintf(`test "$REDIS_URL" = %q`, want)}, 10*time.Second); err != nil {
		t.Fatalf("REDIS_URL not injected correctly: %v", err)
	}
	// The app container must reach the addon through the per-addon
	// network: resolve the addon name via Docker DNS and connect to 6379.
	if err := ts.Docker.ExecProbe(ctx, *dep.ContainerName,
		[]string{"sh", "-c", fmt.Sprintf("nc -z %s 6379", name)}, 15*time.Second); err != nil {
		t.Fatalf("app cannot reach addon over its network: %v", err)
	}
}
