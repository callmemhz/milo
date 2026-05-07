package store

import (
	"context"
	"testing"
)

func TestDeploymentLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateApp(ctx, "myapp", 8080, "/", 30, 0.5, 512)
	u, _ := s.CreateUser(ctx, "alice", false)
	tk, _ := s.CreateUserToken(ctx, u.ID, "h", "")

	d, err := s.CreateDeployment(ctx, a.ID, "sha256:abc", "ghcr.io/x/y@sha256:abc", "c1", "refs/heads/main", tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != "pending" {
		t.Fatalf("status: %s", d.Status)
	}

	if err := s.UpdateDeploymentStatus(ctx, d.ID, "succeeded", "", "app-myapp-1"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetDeployment(ctx, d.ID)
	if got.Status != "succeeded" {
		t.Fatalf("status: %s", got.Status)
	}
	if got.ContainerName == nil || *got.ContainerName != "app-myapp-1" {
		t.Fatal("container name")
	}

	list, _ := s.ListDeploymentsForApp(ctx, a.ID, 10, 0)
	if len(list) != 1 {
		t.Fatalf("len: %d", len(list))
	}
}

func TestListInflightDeployments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateApp(ctx, "myapp", 8080, "/", 30, 0.5, 512)
	u, _ := s.CreateUser(ctx, "alice", false)
	tk, _ := s.CreateUserToken(ctx, u.ID, "h", "")
	_, _ = s.CreateDeployment(ctx, a.ID, "sha256:a", "ref", "", "", tk.ID)
	inflight, _ := s.ListInflightDeployments(ctx)
	if len(inflight) != 1 {
		t.Fatalf("len: %d", len(inflight))
	}
}
