package store

import (
	"context"
	"time"

	"github.com/callmemhz/milo/internal/store/sqlcgen"
)

type User = sqlcgen.User

func (s *Store) CreateUser(ctx context.Context, username string, isAdmin bool) (User, error) {
	return s.Q.CreateUser(ctx, sqlcgen.CreateUserParams{
		Username:  username,
		IsAdmin:   isAdmin,
		CreatedAt: time.Now().UTC(),
	})
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	return s.Q.GetUserByUsername(ctx, username)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (User, error) {
	return s.Q.GetUserByID(ctx, id)
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	return s.Q.ListUsers(ctx)
}

func (s *Store) SoftDeleteUser(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	return s.Q.SoftDeleteUser(ctx, sqlcgen.SoftDeleteUserParams{
		DeletedAt: &now,
		ID:        id,
	})
}

func (s *Store) CountAdmins(ctx context.Context) (int64, error) {
	return s.Q.CountAdmins(ctx)
}
