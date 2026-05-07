package store

import (
	"context"
	"testing"
)

func TestCreateAndFetchUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u, err := s.CreateUser(ctx, "alice", false)
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "alice" || u.IsAdmin {
		t.Fatalf("unexpected user: %+v", u)
	}

	got, err := s.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID {
		t.Fatalf("id mismatch")
	}
}

func TestUsernameUniqueAmongActive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, err := s.CreateUser(ctx, "alice", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateUser(ctx, "alice", false)
	if err == nil {
		t.Fatal("expected uniqueness error")
	}
}

func TestSoftDeleteAllowsReuse(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, "alice", false)
	if err := s.SoftDeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, "alice", false); err != nil {
		t.Fatalf("expected reuse to succeed: %v", err)
	}
}

func TestCountAdmins(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	n, _ := s.CountAdmins(ctx)
	if n != 0 {
		t.Fatalf("want 0 got %d", n)
	}
	_, _ = s.CreateUser(ctx, "admin", true)
	n, _ = s.CountAdmins(ctx)
	if n != 1 {
		t.Fatalf("want 1 got %d", n)
	}
}
