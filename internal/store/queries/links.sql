-- name: CreateLink :one
INSERT INTO links (app_id, addon_id, alias, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetLink :one
SELECT * FROM links WHERE app_id = ? AND addon_id = ?;

-- name: DeleteLink :exec
DELETE FROM links WHERE app_id = ? AND addon_id = ?;

-- name: DeleteLinksForApp :exec
DELETE FROM links WHERE app_id = ?;

-- name: DeleteLinksForAddon :exec
DELETE FROM links WHERE addon_id = ?;

-- name: ListLinksForApp :many
SELECT l.id, l.alias,
       s.id AS addon_id, s.name AS addon_name, s.engine, s.version, s.password
  FROM links l
  INNER JOIN addons s ON s.id = l.addon_id
  WHERE l.app_id = ? AND s.deleted_at IS NULL
  ORDER BY l.id;

-- name: ListLinksForAddon :many
SELECT l.id, l.alias,
       a.id AS app_id, a.name AS app_name, a.current_deploy_id
  FROM links l
  INNER JOIN apps a ON a.id = l.app_id
  WHERE l.addon_id = ? AND a.deleted_at IS NULL
  ORDER BY l.id;
