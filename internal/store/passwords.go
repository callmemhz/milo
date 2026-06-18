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
