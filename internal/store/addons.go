package store

import (
	"context"
	"time"

	"github.com/callmemhz/milo/internal/store/sqlcgen"
)

type Addon = sqlcgen.Addon

// Addon status values. A addon is "provisioning" while its container is
// being created, "running" once the readiness probe passes, and "failed" if
// provisioning errored (or the server crashed mid-provision).
const (
	AddonProvisioning = "provisioning"
	AddonRunning      = "running"
	AddonFailed       = "failed"
)

func (s *Store) CreateAddon(ctx context.Context, name, engine, version string, cpu float64, memMB int64, password string) (Addon, error) {
	return s.Q.CreateAddon(ctx, sqlcgen.CreateAddonParams{
		Name:          name,
		Engine:        engine,
		Version:       version,
		CpuLimit:      cpu,
		MemoryLimitMb: memMB,
		Password:      password,
		CreatedAt:     time.Now().UTC(),
	})
}

func (s *Store) GetAddonByName(ctx context.Context, name string) (Addon, error) {
	return s.Q.GetAddonByName(ctx, name)
}

func (s *Store) GetAddonByID(ctx context.Context, id int64) (Addon, error) {
	return s.Q.GetAddonByID(ctx, id)
}

func (s *Store) ListAddons(ctx context.Context) ([]Addon, error) {
	return s.Q.ListAddons(ctx)
}

func (s *Store) ListAddonsByOwner(ctx context.Context, userID int64) ([]Addon, error) {
	return s.Q.ListAddonsByOwner(ctx, userID)
}

func (s *Store) ListInflightAddons(ctx context.Context) ([]Addon, error) {
	return s.Q.ListInflightAddons(ctx)
}

func (s *Store) UpdateAddonStatus(ctx context.Context, id int64, status, containerName string) error {
	var cn *string
	if containerName != "" {
		cn = &containerName
	}
	return s.Q.UpdateAddonStatus(ctx, sqlcgen.UpdateAddonStatusParams{
		Status:        status,
		ContainerName: cn,
		ID:            id,
	})
}

func (s *Store) SoftDeleteAddon(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	return s.Q.SoftDeleteAddon(ctx, sqlcgen.SoftDeleteAddonParams{
		DeletedAt: &now,
		ID:        id,
	})
}

func (s *Store) AddAddonOwner(ctx context.Context, addonID, userID int64) error {
	return s.Q.AddAddonOwner(ctx, sqlcgen.AddAddonOwnerParams{AddonID: addonID, UserID: userID})
}

func (s *Store) RemoveAddonOwner(ctx context.Context, addonID, userID int64) error {
	return s.Q.RemoveAddonOwner(ctx, sqlcgen.RemoveAddonOwnerParams{AddonID: addonID, UserID: userID})
}

func (s *Store) ListAddonOwners(ctx context.Context, addonID int64) ([]User, error) {
	return s.Q.ListAddonOwners(ctx, addonID)
}

func (s *Store) IsAddonOwner(ctx context.Context, addonID, userID int64) (bool, error) {
	return s.Q.IsAddonOwner(ctx, sqlcgen.IsAddonOwnerParams{AddonID: addonID, UserID: userID})
}
