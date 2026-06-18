-- name: CreateAddon :one
INSERT INTO addons (name, engine, version, cpu_limit, memory_limit_mb, password, status, created_at)
VALUES (?, ?, ?, ?, ?, ?, 'provisioning', ?)
RETURNING *;

-- name: GetAddonByName :one
SELECT * FROM addons WHERE name = ? AND deleted_at IS NULL;

-- name: GetAddonByID :one
SELECT * FROM addons WHERE id = ? AND deleted_at IS NULL;

-- name: ListAddons :many
SELECT * FROM addons WHERE deleted_at IS NULL ORDER BY id;

-- name: ListAddonsByOwner :many
SELECT s.* FROM addons s
  INNER JOIN addon_owners o ON o.addon_id = s.id
  WHERE o.user_id = ? AND s.deleted_at IS NULL
  ORDER BY s.id;

-- name: ListInflightAddons :many
SELECT * FROM addons WHERE status = 'provisioning' AND deleted_at IS NULL;

-- name: UpdateAddonStatus :exec
UPDATE addons SET status = ?, container_name = ? WHERE id = ?;

-- name: SetAddonExposed :exec
UPDATE addons SET exposed = ? WHERE id = ?;

-- name: SetAddonHostPort :exec
UPDATE addons SET host_port = ? WHERE id = ?;

-- name: SoftDeleteAddon :exec
UPDATE addons SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL;

-- name: AddAddonOwner :exec
INSERT INTO addon_owners (addon_id, user_id) VALUES (?, ?);

-- name: RemoveAddonOwner :exec
DELETE FROM addon_owners WHERE addon_id = ? AND user_id = ?;

-- name: ListAddonOwners :many
SELECT u.* FROM users u
  INNER JOIN addon_owners o ON o.user_id = u.id
  WHERE o.addon_id = ? AND u.deleted_at IS NULL ORDER BY u.id;

-- name: IsAddonOwner :one
SELECT EXISTS(SELECT 1 FROM addon_owners WHERE addon_id = ? AND user_id = ?) AS is_owner;
