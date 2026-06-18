package store

import (
	"context"
	"database/sql"
	"time"
)

// SetUserPassword stores a bcrypt hash for the user. Raw SQL (not sqlc) so the
// generated User struct stays unaware of the credential column.
func (s *Store) SetUserPassword(ctx context.Context, userID int64, hash string) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, password_set_at = ? WHERE id = ? AND deleted_at IS NULL`,
		hash, time.Now().UTC(), userID)
	return err
}

// GetUserPasswordHash returns the stored hash (empty string if the user has
// never set a password).
func (s *Store) GetUserPasswordHash(ctx context.Context, userID int64) (string, error) {
	var h sql.NullString
	err := s.DB.QueryRowContext(ctx,
		`SELECT password_hash FROM users WHERE id = ? AND deleted_at IS NULL`, userID).Scan(&h)
	if err != nil {
		return "", err
	}
	return h.String, nil
}

// SetUserFrozen freezes (frozen=true) or unfreezes a user. A frozen account is
// retained but cannot authenticate (login, session, or API token).
func (s *Store) SetUserFrozen(ctx context.Context, userID int64, frozen bool) error {
	if frozen {
		_, err := s.DB.ExecContext(ctx,
			`UPDATE users SET frozen_at = ? WHERE id = ? AND deleted_at IS NULL`,
			time.Now().UTC(), userID)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET frozen_at = NULL WHERE id = ?`, userID)
	return err
}

// IsUserFrozen reports whether the user is currently frozen.
func (s *Store) IsUserFrozen(ctx context.Context, userID int64) (bool, error) {
	var t sql.NullTime
	err := s.DB.QueryRowContext(ctx,
		`SELECT frozen_at FROM users WHERE id = ? AND deleted_at IS NULL`, userID).Scan(&t)
	if err != nil {
		return false, err
	}
	return t.Valid, nil
}
