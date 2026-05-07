package store

import (
	"context"
	"time"

	"github.com/callmemhz/milo-apps-kit/internal/store/sqlcgen"
)

type Token = sqlcgen.Token

func (s *Store) CreateUserToken(ctx context.Context, userID int64, hash, name string) (Token, error) {
	uid := userID
	var namePtr *string
	if name != "" {
		namePtr = &name
	}
	return s.Q.CreateToken(ctx, sqlcgen.CreateTokenParams{
		TokenHash: hash,
		Kind:      "user",
		UserID:    &uid,
		AppID:     nil,
		Name:      namePtr,
		CreatedAt: time.Now().UTC(),
	})
}

func (s *Store) CreateDeployToken(ctx context.Context, appID int64, hash, name string) (Token, error) {
	aid := appID
	var namePtr *string
	if name != "" {
		namePtr = &name
	}
	return s.Q.CreateToken(ctx, sqlcgen.CreateTokenParams{
		TokenHash: hash,
		Kind:      "deploy",
		UserID:    nil,
		AppID:     &aid,
		Name:      namePtr,
		CreatedAt: time.Now().UTC(),
	})
}

func (s *Store) GetTokenByHash(ctx context.Context, hash string) (Token, error) {
	return s.Q.GetTokenByHash(ctx, hash)
}

func (s *Store) ListUserTokens(ctx context.Context, userID int64) ([]Token, error) {
	uid := userID
	return s.Q.ListUserTokens(ctx, &uid)
}

func (s *Store) ListDeployTokens(ctx context.Context, appID int64) ([]Token, error) {
	aid := appID
	return s.Q.ListDeployTokens(ctx, &aid)
}

func (s *Store) RevokeToken(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	return s.Q.RevokeToken(ctx, sqlcgen.RevokeTokenParams{
		RevokedAt: &now,
		ID:        id,
	})
}

func (s *Store) TouchTokenLastUsed(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	return s.Q.TouchTokenLastUsed(ctx, sqlcgen.TouchTokenLastUsedParams{
		LastUsedAt: &now,
		ID:         id,
	})
}
