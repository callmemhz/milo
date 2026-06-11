package store

import (
	"context"
	"testing"
)

func TestLinkLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateApp(ctx, "web", 8080, "/", 30, 0.5, 512)
	addon, _ := s.CreateAddon(ctx, "mydb", "postgres", "16", 0.5, 512, "secret")

	l, err := s.CreateLink(ctx, a.ID, addon.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if l.AppID != a.ID || l.AddonID != addon.ID {
		t.Fatalf("unexpected: %+v", l)
	}

	// duplicate link rejected
	if _, err := s.CreateLink(ctx, a.ID, addon.ID, "X"); err == nil {
		t.Fatal("expected duplicate error")
	}

	fromApp, err := s.ListLinksForApp(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fromApp) != 1 || fromApp[0].AddonName != "mydb" || fromApp[0].Password != "secret" {
		t.Fatalf("unexpected: %+v", fromApp)
	}

	fromAddon, err := s.ListLinksForAddon(ctx, addon.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fromAddon) != 1 || fromAddon[0].AppName != "web" {
		t.Fatalf("unexpected: %+v", fromAddon)
	}

	if err := s.DeleteLink(ctx, a.ID, addon.ID); err != nil {
		t.Fatal(err)
	}
	fromApp, _ = s.ListLinksForApp(ctx, a.ID)
	if len(fromApp) != 0 {
		t.Fatal("expected no links after delete")
	}
}

func TestLinksHideSoftDeletedPeers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateApp(ctx, "web", 8080, "/", 30, 0.5, 512)
	addon, _ := s.CreateAddon(ctx, "mydb", "postgres", "16", 0.5, 512, "secret")
	_, _ = s.CreateLink(ctx, a.ID, addon.ID, "")

	// soft-deleted addon disappears from the app's link list
	_ = s.SoftDeleteAddon(ctx, addon.ID)
	fromApp, _ := s.ListLinksForApp(ctx, a.ID)
	if len(fromApp) != 0 {
		t.Fatalf("expected no links, got %+v", fromApp)
	}

	// soft-deleted app disappears from the addon's link list
	addon2, _ := s.CreateAddon(ctx, "mydb2", "redis", "7", 0.5, 512, "x")
	_, _ = s.CreateLink(ctx, a.ID, addon2.ID, "")
	_ = s.SoftDeleteApp(ctx, a.ID)
	fromAddon, _ := s.ListLinksForAddon(ctx, addon2.ID)
	if len(fromAddon) != 0 {
		t.Fatalf("expected no links, got %+v", fromAddon)
	}
}
