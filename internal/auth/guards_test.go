package auth

import (
	"context"
	"testing"

	"github.com/callmemhz/milo/internal/store"
)

func TestRequireAdmin(t *testing.T) {
	if RequireAdmin(nil) == nil {
		t.Fatal("nil should fail")
	}
	if RequireAdmin(&Identity{User: &store.User{IsAdmin: false}}) == nil {
		t.Fatal("non-admin should fail")
	}
	if err := RequireAdmin(&Identity{User: &store.User{IsAdmin: true}}); err != nil {
		t.Fatalf("admin should pass: %v", err)
	}
}

func TestRequireOwnerOrAdmin(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	admin, _ := s.CreateUser(ctx, "admin", true)
	owner, _ := s.CreateUser(ctx, "alice", false)
	other, _ := s.CreateUser(ctx, "bob", false)
	a, _ := s.CreateApp(ctx, "myapp", 8080, "/", 30, 0.5, 512)
	_ = s.AddOwner(ctx, a.ID, owner.ID)

	if err := RequireOwnerOrAdmin(ctx, s, &Identity{User: &admin}, a.ID); err != nil {
		t.Fatalf("admin should pass: %v", err)
	}
	if err := RequireOwnerOrAdmin(ctx, s, &Identity{User: &owner}, a.ID); err != nil {
		t.Fatalf("owner should pass: %v", err)
	}
	if err := RequireOwnerOrAdmin(ctx, s, &Identity{User: &other}, a.ID); err == nil {
		t.Fatal("non-owner should fail")
	}
}

func TestRequireDeployScope(t *testing.T) {
	appID := int64(7)
	other := int64(8)
	if err := RequireDeployScope(&Identity{AppID: &appID}, 7); err != nil {
		t.Fatal("scope match should pass")
	}
	if err := RequireDeployScope(&Identity{AppID: &appID}, other); err == nil {
		t.Fatal("scope mismatch should fail")
	}
	if err := RequireDeployScope(nil, 7); err == nil {
		t.Fatal("nil should fail")
	}
	user := store.User{}
	if err := RequireDeployScope(&Identity{User: &user, AppID: &appID}, 7); err == nil {
		t.Fatal("user-bearing identity should be rejected by deploy guard")
	}
}
