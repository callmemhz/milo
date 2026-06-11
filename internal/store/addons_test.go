package store

import (
	"context"
	"testing"
)

func TestCreateAndGetAddon(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	addon, err := s.CreateAddon(ctx, "mydb", "postgres", "16", 0.5, 512, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if addon.Engine != "postgres" || addon.Status != AddonProvisioning {
		t.Fatalf("unexpected: %+v", addon)
	}

	got, err := s.GetAddonByName(ctx, "mydb")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != addon.ID || got.Password != "secret" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestAddonNameUniqueAmongActive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateAddon(ctx, "mydb", "postgres", "16", 0.5, 512, "a")
	if _, err := s.CreateAddon(ctx, "mydb", "redis", "7", 0.5, 512, "b"); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestAddonEngineConstraint(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateAddon(ctx, "mydb", "mysql", "8", 0.5, 512, "a"); err == nil {
		t.Fatal("expected engine check violation")
	}
}

func TestSoftDeleteAllowsAddonNameReuse(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	addon, _ := s.CreateAddon(ctx, "mydb", "postgres", "16", 0.5, 512, "a")
	if err := s.SoftDeleteAddon(ctx, addon.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAddonByName(ctx, "mydb"); err == nil {
		t.Fatal("expected not found after soft delete")
	}
	if _, err := s.CreateAddon(ctx, "mydb", "redis", "7", 0.5, 512, "b"); err != nil {
		t.Fatal(err)
	}
}

func TestAddonStatusAndOwners(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	addon, _ := s.CreateAddon(ctx, "mydb", "postgres", "16", 0.5, 512, "a")
	if err := s.UpdateAddonStatus(ctx, addon.ID, AddonRunning, "addon-mydb"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetAddonByID(ctx, addon.ID)
	if got.Status != AddonRunning || got.ContainerName == nil || *got.ContainerName != "addon-mydb" {
		t.Fatalf("unexpected: %+v", got)
	}

	u, _ := s.CreateUser(ctx, "alice", false)
	if err := s.AddAddonOwner(ctx, addon.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	ok, _ := s.IsAddonOwner(ctx, addon.ID, u.ID)
	if !ok {
		t.Fatal("expected owner")
	}
	owned, _ := s.ListAddonsByOwner(ctx, u.ID)
	if len(owned) != 1 || owned[0].ID != addon.ID {
		t.Fatalf("unexpected: %+v", owned)
	}
}

func TestListInflightAddons(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateAddon(ctx, "db1", "postgres", "16", 0.5, 512, "a")
	b, _ := s.CreateAddon(ctx, "db2", "redis", "7", 0.5, 512, "b")
	_ = s.UpdateAddonStatus(ctx, b.ID, AddonRunning, "addon-db2")
	inflight, err := s.ListInflightAddons(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(inflight) != 1 || inflight[0].ID != a.ID {
		t.Fatalf("unexpected: %+v", inflight)
	}
}
