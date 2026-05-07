-- name: GetAppEnv :many
SELECT key, value FROM app_env WHERE app_id = ? ORDER BY key;

-- name: SetAppEnvVar :exec
INSERT INTO app_env (app_id, key, value) VALUES (?, ?, ?)
  ON CONFLICT(app_id, key) DO UPDATE SET value = excluded.value;

-- name: DeleteAppEnvVar :exec
DELETE FROM app_env WHERE app_id = ? AND key = ?;

-- name: DeleteAllAppEnv :exec
DELETE FROM app_env WHERE app_id = ?;
