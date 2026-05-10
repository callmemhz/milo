package store

import (
	"context"
	"testing"
)

func TestCreateAndGetApp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, err := s.CreateApp(ctx, "myapp", AppConfig{Port: 8080, HealthPath: "/", HealthTimeoutSec: 30, CPULimit: 0.5, MemoryLimitMB: 512})
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "myapp" || a.Port != 8080 {
		t.Fatalf("unexpected: %+v", a)
	}

	got, err := s.GetAppByName(ctx, "myapp")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != a.ID {
		t.Fatal("id mismatch")
	}
}

func TestAppNameUniqueAmongActive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateApp(ctx, "myapp", AppConfig{Port: 8080, HealthPath: "/", HealthTimeoutSec: 30, CPULimit: 0.5, MemoryLimitMB: 512})
	if _, err := s.CreateApp(ctx, "myapp", AppConfig{Port: 8080, HealthPath: "/", HealthTimeoutSec: 30, CPULimit: 0.5, MemoryLimitMB: 512}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestSoftDeleteAllowsAppNameReuse(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateApp(ctx, "myapp", AppConfig{Port: 8080, HealthPath: "/", HealthTimeoutSec: 30, CPULimit: 0.5, MemoryLimitMB: 512})
	_ = s.SoftDeleteApp(ctx, a.ID)
	if _, err := s.CreateApp(ctx, "myapp", AppConfig{Port: 8080, HealthPath: "/", HealthTimeoutSec: 30, CPULimit: 0.5, MemoryLimitMB: 512}); err != nil {
		t.Fatalf("reuse failed: %v", err)
	}
}

func TestOwners(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateApp(ctx, "myapp", AppConfig{Port: 8080, HealthPath: "/", HealthTimeoutSec: 30, CPULimit: 0.5, MemoryLimitMB: 512})
	u, _ := s.CreateUser(ctx, "alice", false)
	if err := s.AddOwner(ctx, a.ID, u.ID); err != nil {
		t.Fatal(err)
	}

	owners, err := s.ListOwners(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(owners) != 1 || owners[0].ID != u.ID {
		t.Fatalf("owners: %+v", owners)
	}

	yes, _ := s.IsOwner(ctx, a.ID, u.ID)
	if !yes {
		t.Fatal("expected IsOwner true")
	}

	apps, _ := s.ListAppsByOwner(ctx, u.ID)
	if len(apps) != 1 || apps[0].ID != a.ID {
		t.Fatalf("apps: %+v", apps)
	}

	_ = s.RemoveOwner(ctx, a.ID, u.ID)
	yes, _ = s.IsOwner(ctx, a.ID, u.ID)
	if yes {
		t.Fatal("expected IsOwner false after removal")
	}
}

func TestUpdateAppConfig(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateApp(ctx, "myapp", AppConfig{Port: 8080, HealthPath: "/", HealthTimeoutSec: 30, CPULimit: 0.5, MemoryLimitMB: 512})
	cfg := AppConfig{Port: 9000, HealthPath: "/healthz", HealthTimeoutSec: 60, CPULimit: 1.0, MemoryLimitMB: 1024}
	if err := s.UpdateAppConfig(ctx, a.ID, cfg); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetAppByID(ctx, a.ID)
	if got.Port != 9000 || got.HealthPath != "/healthz" || got.MemoryLimitMb != 1024 {
		t.Fatalf("update failed: %+v", got)
	}
}

func TestSetCurrentDeploy(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateApp(ctx, "myapp", AppConfig{Port: 8080, HealthPath: "/", HealthTimeoutSec: 30, CPULimit: 0.5, MemoryLimitMB: 512})
	if a.CurrentDeployID != nil {
		t.Fatal("CurrentDeployID should be nil initially")
	}
	// Create a placeholder deploy via direct SQL? — defer to deployments tests.
}

// Deferred from M2 (depends on CreateApp):
func TestDeployTokenScope(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateApp(ctx, "myapp", AppConfig{Port: 8080, HealthPath: "/", HealthTimeoutSec: 30, CPULimit: 0.5, MemoryLimitMB: 512})
	tk, err := s.CreateDeployToken(ctx, a.ID, "hash-d", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Kind != "deploy" || tk.AppID == nil || *tk.AppID != a.ID {
		t.Fatalf("unexpected: %+v", tk)
	}
}
