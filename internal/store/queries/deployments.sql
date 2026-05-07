-- name: CreateDeployment :one
INSERT INTO deployments (app_id, image_digest, image_ref, commit_sha, ref, status, triggered_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetDeployment :one
SELECT * FROM deployments WHERE id = ?;

-- name: ListDeploymentsForApp :many
SELECT * FROM deployments WHERE app_id = ? ORDER BY id DESC LIMIT ? OFFSET ?;

-- name: UpdateDeploymentStatus :exec
UPDATE deployments SET status = ?, failure_reason = ?, container_name = ?, finished_at = ?
  WHERE id = ?;

-- name: ListInflightDeployments :many
SELECT * FROM deployments WHERE status IN ('pending','deploying');
