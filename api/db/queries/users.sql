-- name: CreateUser :one
INSERT INTO users (email, display_name, role)
VALUES (lower(sqlc.arg(email)), sqlc.arg(display_name), sqlc.arg(role))
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetActiveUserByEmail :one
-- Identité lue dans le JWT Cloudflare Access.
SELECT * FROM users WHERE email = lower(sqlc.arg(email)) AND is_active;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at;

-- name: SetUserActive :exec
UPDATE users SET is_active = $2, updated_at = now() WHERE id = $1;

-- name: DeleteUser :exec
-- Supprime l'utilisateur et toutes ses données (EF-30, cascade).
DELETE FROM users WHERE id = $1;

-- name: CreateDefaultSettings :one
INSERT INTO user_settings (user_id) VALUES ($1) RETURNING *;

-- name: GetSettings :one
SELECT * FROM user_settings WHERE user_id = $1;

-- name: UpdateSettings :one
UPDATE user_settings
SET step_cents      = sqlc.arg(step_cents),
    multiplier      = sqlc.arg(multiplier),
    tx_cap_cents    = sqlc.arg(tx_cap_cents),
    month_cap_cents = sqlc.narg(month_cap_cents),
    updated_at      = now()
WHERE user_id = sqlc.arg(user_id)
RETURNING *;
