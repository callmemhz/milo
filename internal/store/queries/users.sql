-- name: CreateUser :one
INSERT INTO users (username, is_admin, created_at)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ? AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = ? AND deleted_at IS NULL;

-- name: ListUsers :many
SELECT * FROM users WHERE deleted_at IS NULL ORDER BY id;

-- name: SoftDeleteUser :exec
UPDATE users SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL;

-- name: CountAdmins :one
SELECT COUNT(*) FROM users WHERE is_admin = 1 AND deleted_at IS NULL;
