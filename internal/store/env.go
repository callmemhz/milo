package store

import (
	"context"

	"github.com/callmemhz/milo-apps-kit/internal/store/sqlcgen"
)

func (s *Store) GetAppEnv(ctx context.Context, appID int64) (map[string]string, error) {
	rows, err := s.Q.GetAppEnv(ctx, appID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

func (s *Store) SetAppEnvVar(ctx context.Context, appID int64, key, value string) error {
	return s.Q.SetAppEnvVar(ctx, sqlcgen.SetAppEnvVarParams{AppID: appID, Key: key, Value: value})
}

func (s *Store) DeleteAppEnvVar(ctx context.Context, appID int64, key string) error {
	return s.Q.DeleteAppEnvVar(ctx, sqlcgen.DeleteAppEnvVarParams{AppID: appID, Key: key})
}

func (s *Store) ReplaceAppEnv(ctx context.Context, appID int64, env map[string]string) error {
	return s.WithTx(ctx, func(q *sqlcgen.Queries) error {
		if err := q.DeleteAllAppEnv(ctx, appID); err != nil {
			return err
		}
		for k, v := range env {
			if err := q.SetAppEnvVar(ctx, sqlcgen.SetAppEnvVarParams{AppID: appID, Key: k, Value: v}); err != nil {
				return err
			}
		}
		return nil
	})
}
