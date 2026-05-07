package store

import (
	"context"
	"time"

	"github.com/callmemhz/milo-apps-kit/internal/store/sqlcgen"
)

type App = sqlcgen.App

type AppConfig struct {
	Port             int64
	HealthPath       string
	HealthTimeoutSec int64
	CPULimit         float64
	MemoryLimitMB    int64
}

func (s *Store) CreateApp(ctx context.Context, name string, port int64, healthPath string, timeoutSec int64, cpu float64, memMB int64) (App, error) {
	return s.Q.CreateApp(ctx, sqlcgen.CreateAppParams{
		Name:             name,
		Port:             port,
		HealthPath:       healthPath,
		HealthTimeoutSec: timeoutSec,
		CpuLimit:         cpu,
		MemoryLimitMb:    memMB,
		CreatedAt:        time.Now().UTC(),
	})
}

func (s *Store) GetAppByName(ctx context.Context, name string) (App, error) {
	return s.Q.GetAppByName(ctx, name)
}

func (s *Store) GetAppByID(ctx context.Context, id int64) (App, error) {
	return s.Q.GetAppByID(ctx, id)
}

func (s *Store) ListApps(ctx context.Context) ([]App, error) {
	return s.Q.ListApps(ctx)
}

func (s *Store) ListAppsByOwner(ctx context.Context, userID int64) ([]App, error) {
	return s.Q.ListAppsByOwner(ctx, userID)
}

func (s *Store) UpdateAppConfig(ctx context.Context, id int64, c AppConfig) error {
	return s.Q.UpdateAppConfig(ctx, sqlcgen.UpdateAppConfigParams{
		Port:             c.Port,
		HealthPath:       c.HealthPath,
		HealthTimeoutSec: c.HealthTimeoutSec,
		CpuLimit:         c.CPULimit,
		MemoryLimitMb:    c.MemoryLimitMB,
		ID:               id,
	})
}

func (s *Store) SetCurrentDeploy(ctx context.Context, appID, deployID int64) error {
	did := deployID
	return s.Q.SetCurrentDeploy(ctx, sqlcgen.SetCurrentDeployParams{
		CurrentDeployID: &did,
		ID:              appID,
	})
}

func (s *Store) SoftDeleteApp(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	return s.Q.SoftDeleteApp(ctx, sqlcgen.SoftDeleteAppParams{
		DeletedAt: &now,
		ID:        id,
	})
}

func (s *Store) AddOwner(ctx context.Context, appID, userID int64) error {
	return s.Q.AddOwner(ctx, sqlcgen.AddOwnerParams{AppID: appID, UserID: userID})
}

func (s *Store) RemoveOwner(ctx context.Context, appID, userID int64) error {
	return s.Q.RemoveOwner(ctx, sqlcgen.RemoveOwnerParams{AppID: appID, UserID: userID})
}

func (s *Store) ListOwners(ctx context.Context, appID int64) ([]User, error) {
	return s.Q.ListOwners(ctx, appID)
}

func (s *Store) IsOwner(ctx context.Context, appID, userID int64) (bool, error) {
	return s.Q.IsOwner(ctx, sqlcgen.IsOwnerParams{AppID: appID, UserID: userID})
}
