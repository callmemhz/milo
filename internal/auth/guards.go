package auth

import (
	"context"
	"errors"

	"github.com/callmemhz/milo/internal/store"
)

var (
	ErrNotAdmin   = errors.New("admin only")
	ErrNotOwner   = errors.New("not an owner")
	ErrWrongScope = errors.New("wrong token scope")
)

func RequireAdmin(id *Identity) error {
	if id == nil || id.User == nil || !id.User.IsAdmin {
		return ErrNotAdmin
	}
	return nil
}

func RequireOwnerOrAdmin(ctx context.Context, s *store.Store, id *Identity, appID int64) error {
	if id == nil || id.User == nil {
		return ErrNotOwner
	}
	if id.User.IsAdmin {
		return nil
	}
	yes, err := s.IsOwner(ctx, appID, id.User.ID)
	if err != nil {
		return err
	}
	if !yes {
		return ErrNotOwner
	}
	return nil
}

func RequireDeployScope(id *Identity, appID int64) error {
	if id == nil {
		return ErrWrongScope
	}
	// User-token holders should be checked via RequireOwnerOrAdmin; this guard is for deploy tokens.
	if id.User != nil {
		return ErrWrongScope
	}
	if id.AppID == nil || *id.AppID != appID {
		return ErrWrongScope
	}
	return nil
}
