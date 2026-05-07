package bootstrap

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/callmemhz/milo-apps-kit/internal/store"
)

func captureStderr(fn func()) string {
	orig := os.Stderr
	rd, wr, _ := os.Pipe()
	os.Stderr = wr
	fn()
	wr.Close()
	os.Stderr = orig
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, rd)
	return buf.String()
}

func openMem(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open("file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestEnsureAdminFirstRunCreatesUserAndPrintsToken(t *testing.T) {
	s := openMem(t)
	out := captureStderr(func() {
		if err := EnsureAdmin(context.Background(), s, slog.Default()); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "BOOTSTRAP_ADMIN_TOKEN=") {
		t.Fatalf("stderr missing marker: %q", out)
	}
	n, _ := s.CountAdmins(context.Background())
	if n != 1 {
		t.Fatalf("admins: %d", n)
	}
}

func TestEnsureAdminIdempotent(t *testing.T) {
	s := openMem(t)
	if err := EnsureAdmin(context.Background(), s, slog.Default()); err != nil {
		t.Fatal(err)
	}
	out := captureStderr(func() {
		if err := EnsureAdmin(context.Background(), s, slog.Default()); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(out, "BOOTSTRAP_ADMIN_TOKEN=") {
		t.Fatal("token printed on second run; should be no-op")
	}
	n, _ := s.CountAdmins(context.Background())
	if n != 1 {
		t.Fatalf("admins: %d", n)
	}
}
