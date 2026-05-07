package store

import (
	"context"
	"testing"
)

func TestCreateAndGetUserToken(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, "alice", false)

	tk, err := s.CreateUserToken(ctx, u.ID, "hash-1", "ci")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Kind != "user" {
		t.Fatalf("kind: %s", tk.Kind)
	}

	got, err := s.GetTokenByHash(ctx, "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != tk.ID {
		t.Fatal("id mismatch")
	}
}

func TestRevokeMakesTokenInvisible(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, "alice", false)
	tk, _ := s.CreateUserToken(ctx, u.ID, "hash-2", "")
	_ = s.RevokeToken(ctx, tk.ID)
	if _, err := s.GetTokenByHash(ctx, "hash-2"); err == nil {
		t.Fatal("expected lookup to fail after revoke")
	}
}

func TestListUserTokensFiltersRevoked(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, "alice", false)
	tk1, _ := s.CreateUserToken(ctx, u.ID, "h1", "a")
	_, _ = s.CreateUserToken(ctx, u.ID, "h2", "b")
	_ = s.RevokeToken(ctx, tk1.ID)

	list, err := s.ListUserTokens(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 active token, got %d", len(list))
	}
}

func TestTouchTokenLastUsed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, "alice", false)
	tk, _ := s.CreateUserToken(ctx, u.ID, "h", "")
	if tk.LastUsedAt != nil {
		t.Fatal("LastUsedAt should be nil initially")
	}
	if err := s.TouchTokenLastUsed(ctx, tk.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTokenByHash(ctx, "h")
	if got.LastUsedAt == nil {
		t.Fatal("LastUsedAt should be set after touch")
	}
}
