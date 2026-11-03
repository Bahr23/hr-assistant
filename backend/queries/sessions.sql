-- name: CreateSession :one
INSERT INTO sessions (id, user_id, refresh_token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1;

-- name: RotateSession :one
UPDATE sessions
SET refresh_token_hash = $3, expires_at = $4, updated_at = now()
WHERE id = $1
  AND refresh_token_hash = $2
  AND revoked_at IS NULL
  AND expires_at > now()
RETURNING *;

-- name: RevokeSession :exec
UPDATE sessions
SET revoked_at = COALESCE(revoked_at, now()), updated_at = now()
WHERE id = $1;

