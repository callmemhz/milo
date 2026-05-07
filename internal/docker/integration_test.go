//go:build docker_integration

package docker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	netName := fmt.Sprintf("milo-apps-kit-net-test-%d", time.Now().UnixNano())
	c, err := New(Config{Network: netName})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = c.RemoveNetwork(context.Background(), netName)
		c.Close()
	})
	return c
}

// ---------------------------------------------------------------------------
// 8.1 tests
// ---------------------------------------------------------------------------

func TestEnsureNetwork(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	if err := c.EnsureNetwork(ctx); err != nil {
		t.Fatal(err)
	}
	// Idempotent second call.
	if err := c.EnsureNetwork(ctx); err != nil {
		t.Fatal("idempotent ensure failed:", err)
	}
}

func TestPullSmallImage(t *testing.T) {
	c := newTestClient(t)
	digest, err := c.Pull(context.Background(), "alpine:3.19")
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" {
		t.Fatal("empty digest")
	}
	t.Logf("digest: %s", digest)
}

// ---------------------------------------------------------------------------
// 8.2 tests
// ---------------------------------------------------------------------------

func TestRunStopRemove(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	if err := c.EnsureNetwork(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Pull(ctx, "alpine:3.19"); err != nil {
		t.Fatal(err)
	}

	name := fmt.Sprintf("milo-apps-kit-test-run-%d", time.Now().UnixNano())
	_, err := c.Run(ctx, RunSpec{
		Name:     name,
		Alias:    "test-run",
		Image:    "alpine:3.19",
		Cmd:      []string{"sleep", "60"},
		CPULimit: 0.1,
		MemoryMB: 64,
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := c.InspectByName(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if info.State != "running" {
		t.Fatalf("expected running, got %s", info.State)
	}

	if err := c.Stop(ctx, name, 5); err != nil {
		t.Fatal(err)
	}
	if err := c.Remove(ctx, name); err != nil {
		t.Fatal(err)
	}

	_, err = c.InspectByName(ctx, name)
	if err == nil {
		t.Fatal("expected error after remove, got nil")
	}
}

func TestLogs(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	if err := c.EnsureNetwork(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Pull(ctx, "alpine:3.19"); err != nil {
		t.Fatal(err)
	}

	name := fmt.Sprintf("milo-apps-kit-test-logs-%d", time.Now().UnixNano())
	_, err := c.Run(ctx, RunSpec{
		Name:     name,
		Alias:    "test-logs",
		Image:    "alpine:3.19",
		Cmd:      []string{"sh", "-c", "echo hello && sleep 5"},
		CPULimit: 0.1,
		MemoryMB: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop(ctx, name, 2); _ = c.Remove(ctx, name) })

	// Give the container a moment to produce output.
	time.Sleep(500 * time.Millisecond)

	rc, err := c.Logs(ctx, name, false, "all")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	var buf strings.Builder
	data := make([]byte, 4096)
	for {
		n, err2 := rc.Read(data)
		if n > 0 {
			buf.Write(data[:n])
		}
		if err2 != nil {
			break
		}
	}

	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("expected 'hello' in logs, got: %q", buf.String())
	}
}

func TestListOnNetwork(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	if err := c.EnsureNetwork(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Pull(ctx, "alpine:3.19"); err != nil {
		t.Fatal(err)
	}

	name := fmt.Sprintf("milo-apps-kit-test-list-%d", time.Now().UnixNano())
	_, err := c.Run(ctx, RunSpec{
		Name:     name,
		Alias:    "test-list",
		Image:    "alpine:3.19",
		Cmd:      []string{"sleep", "60"},
		CPULimit: 0.1,
		MemoryMB: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop(ctx, name, 2); _ = c.Remove(ctx, name) })

	containers, err := c.ListOnNetwork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ctr := range containers {
		for _, n := range ctr.Names {
			if strings.TrimPrefix(n, "/") == name {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("container %s not found in network listing", name)
	}
}

func TestIPInNetwork(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	if err := c.EnsureNetwork(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Pull(ctx, "alpine:3.19"); err != nil {
		t.Fatal(err)
	}

	name := fmt.Sprintf("milo-apps-kit-test-ip-%d", time.Now().UnixNano())
	_, err := c.Run(ctx, RunSpec{
		Name:     name,
		Alias:    "test-ip",
		Image:    "alpine:3.19",
		Cmd:      []string{"sleep", "60"},
		CPULimit: 0.1,
		MemoryMB: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop(ctx, name, 2); _ = c.Remove(ctx, name) })

	ip, err := c.IPInNetwork(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if ip == "" {
		t.Fatal("expected non-empty IP")
	}
	t.Logf("container IP: %s", ip)
}

// ---------------------------------------------------------------------------
// 8.3 tests
// ---------------------------------------------------------------------------

func TestHealthCheckPasses(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	if err := c.EnsureNetwork(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Pull(ctx, "nginx:alpine"); err != nil {
		t.Fatal(err)
	}

	name := fmt.Sprintf("hc-test-%d", time.Now().UnixNano())
	_, err := c.Run(ctx, RunSpec{
		Name:        name,
		Alias:       "hc-test",
		Image:       "nginx:alpine",
		Port:        80,
		PublishPort: true,
		CPULimit:    0.5,
		MemoryMB:    128,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop(ctx, name, 2); _ = c.Remove(ctx, name) })

	info, err := c.InspectByName(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if info.HostPort == 0 {
		t.Fatal("expected host port mapping")
	}

	if err := c.HealthCheck(ctx, "localhost", info.HostPort, "/", name, 30*time.Second); err != nil {
		t.Fatalf("health: %v", err)
	}
}

func TestHealthCheckFailsOnExit(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	if err := c.EnsureNetwork(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Pull(ctx, "alpine:3.19"); err != nil {
		t.Fatal(err)
	}

	name := fmt.Sprintf("hc-exit-%d", time.Now().UnixNano())
	_, err := c.Run(ctx, RunSpec{
		Name:        name,
		Alias:       "hc-exit",
		Image:       "alpine:3.19",
		Cmd:         []string{"sh", "-c", "exit 1"},
		Port:        8080,
		PublishPort: true,
		CPULimit:    0.1,
		MemoryMB:    64,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Remove(ctx, name) })

	err = c.HealthCheck(ctx, "localhost", 8080, "/", name, 5*time.Second)
	if err == nil {
		t.Fatal("expected failure from health check")
	}
	t.Logf("got expected error: %v", err)
}

// ---------------------------------------------------------------------------
// 8.4 tests
// ---------------------------------------------------------------------------

func TestVolumeLifecycle(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	name := fmt.Sprintf("milo-apps-kit-test-vol-%d", time.Now().UnixNano())
	if err := c.EnsureVolume(ctx, name); err != nil {
		t.Fatal(err)
	}
	// Second create must be idempotent.
	if err := c.EnsureVolume(ctx, name); err != nil {
		t.Fatalf("second create should be idempotent: %v", err)
	}
	if err := c.RemoveVolume(ctx, name, true); err != nil {
		t.Fatal(err)
	}
}
