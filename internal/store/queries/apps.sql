-- name: CreateApp :one
INSERT INTO apps (name, port, health_path, health_timeout_sec, cpu_limit, memory_limit_mb, volumes, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAppByName :one
SELECT * FROM apps WHERE name = ? AND deleted_at IS NULL;

-- name: GetAppByID :one
SELECT * FROM apps WHERE id = ? AND deleted_at IS NULL;

-- name: ListApps :many
SELECT * FROM apps WHERE deleted_at IS NULL ORDER BY id;

-- name: ListAppsByOwner :many
SELECT a.* FROM apps a
  INNER JOIN app_owners o ON o.app_id = a.id
  WHERE o.user_id = ? AND a.deleted_at IS NULL
  ORDER BY a.id;

-- name: UpdateAppConfig :exec
UPDATE apps SET port = ?, health_path = ?, health_timeout_sec = ?, cpu_limit = ?, memory_limit_mb = ?, volumes = ?
  WHERE id = ?;

-- name: SetCurrentDeploy :exec
UPDATE apps SET current_deploy_id = ? WHERE id = ?;

-- name: SoftDeleteApp :exec
UPDATE apps SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL;

-- name: AddOwner :exec
INSERT INTO app_owners (app_id, user_id) VALUES (?, ?);

-- name: RemoveOwner :exec
DELETE FROM app_owners WHERE app_id = ? AND user_id = ?;

-- name: ListOwners :many
SELECT u.* FROM users u
  INNER JOIN app_owners o ON o.user_id = u.id
  WHERE o.app_id = ? AND u.deleted_at IS NULL ORDER BY u.id;

-- name: IsOwner :one
SELECT EXISTS(SELECT 1 FROM app_owners WHERE app_id = ? AND user_id = ?) AS is_owner;
