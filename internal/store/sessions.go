package store

import (
	"context"
	"time"
)

// Session is a server-side browser session for the web console. The cookie
// carries a random opaque id; the database stores only its sha256 hash, mirroring
// the tokens table. Sessions are revocable (logout, delete user) and expirable.
type Session struct {
	ID         int64
	TokenHash  string
	UserID     int64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt *time.Time
}

func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash, userID, time.Now().UTC(), expiresAt.UTC())
	return err
}

func (s *Store) GetSessionByHash(ctx context.Context, tokenHash string) (Session, error) {
	var ss Session
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, token_hash, user_id, created_at, expires_at, last_seen_at FROM sessions WHERE token_hash = ?`,
		tokenHash).Scan(&ss.ID, &ss.TokenHash, &ss.UserID, &ss.CreatedAt, &ss.ExpiresAt, &ss.LastSeenAt)
	return ss, err
}

// RefreshSession slides the expiry forward and records last_seen. Best-effort.
func (s *Store) RefreshSession(ctx context.Context, id int64, expiresAt time.Time) error {
	now := time.Now().UTC()
	_, err := s.DB.ExecContext(ctx,
		`UPDATE sessions SET expires_at = ?, last_seen_at = ? WHERE id = ?`,
		expiresAt.UTC(), now, id)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, time.Now().UTC())
	return err
}
