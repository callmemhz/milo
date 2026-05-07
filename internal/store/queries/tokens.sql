-- name: CreateToken :one
INSERT INTO tokens (token_hash, kind, user_id, app_id, name, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetTokenByHash :one
SELECT * FROM tokens WHERE token_hash = ? AND revoked_at IS NULL;

-- name: GetTokenByID :one
SELECT * FROM tokens WHERE id = ?;

-- name: ListUserTokens :many
SELECT * FROM tokens WHERE user_id = ? AND revoked_at IS NULL ORDER BY id;

-- name: ListDeployTokens :many
SELECT * FROM tokens WHERE app_id = ? AND revoked_at IS NULL ORDER BY id;

-- name: RevokeToken :exec
UPDATE tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL;

-- name: TouchTokenLastUsed :exec
UPDATE tokens SET last_used_at = ? WHERE id = ?;
