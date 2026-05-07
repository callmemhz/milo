package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/callmemhz/milo-apps-kit/internal/auth"
	"github.com/callmemhz/milo-apps-kit/internal/store"
)

// EnsureAdmin checks whether any admin exists. If not, generates a random token,
// creates an "admin" user, persists the token's hash, and prints the plaintext
// token to STDERR exactly once with the marker `BOOTSTRAP_ADMIN_TOKEN=...`.
// Idempotent: if an admin exists, it's a no-op.
func EnsureAdmin(ctx context.Context, s *store.Store, log *slog.Logger) error {
	n, err := s.CountAdmins(ctx)
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if n > 0 {
		return nil
	}

	plaintext, err := auth.Generate()
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}

	u, err := s.CreateUser(ctx, "admin", true)
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	if _, err := s.CreateUserToken(ctx, u.ID, auth.Hash(plaintext), "bootstrap"); err != nil {
		return fmt.Errorf("create bootstrap token: %w", err)
	}
	// The plaintext is shown to operator exactly once via stderr.
	fmt.Fprintf(os.Stderr, "BOOTSTRAP_ADMIN_TOKEN=%s\n", plaintext)
	log.Info("bootstrap admin user created", "username", "admin")
	return nil
}
