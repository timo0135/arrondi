-- name: GetConnectionByUser :one
SELECT * FROM bank_connections WHERE user_id = $1;

-- name: ActivateConnection :one
-- Fin du callback : session créée. activated_at n'est fixée qu'au premier succès.
UPDATE bank_connections
SET eb_session_id         = sqlc.arg(eb_session_id),
    consent_valid_until   = sqlc.arg(consent_valid_until),
    activated_at          = COALESCE(activated_at, now()),
    status                = 'active',
    auth_state            = NULL,
    auth_state_expires_at = NULL,
    last_error            = NULL,
    updated_at            = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetConnectionStatus :exec
UPDATE bank_connections
SET status = sqlc.arg(status), last_error = sqlc.narg(last_error), updated_at = now()
WHERE id = sqlc.arg(id);

-- name: ListConnectionsToSync :many
-- Connexions actives dont le consentement est encore valable.
SELECT * FROM bank_connections
WHERE status = 'active' AND consent_valid_until > now()
ORDER BY last_sync_at NULLS FIRST;

-- name: ExpireConsents :many
-- Passe en "expired" les connexions dont le consentement est dépassé.
UPDATE bank_connections
SET status = 'expired', updated_at = now()
WHERE status = 'active' AND consent_valid_until <= now()
RETURNING id, user_id;

-- name: RecordSyncAttempt :exec
UPDATE bank_connections
SET last_sync_at    = now(),
    last_sync_ok_at = CASE WHEN sqlc.arg(ok)::boolean THEN now() ELSE last_sync_ok_at END,
    last_error      = sqlc.narg(last_error),
    updated_at      = now()
WHERE id = sqlc.arg(id);

-- name: UpsertAccount :one
INSERT INTO bank_accounts (connection_id, eb_account_uid, iban_last4, label, currency)
VALUES (sqlc.arg(connection_id), sqlc.arg(eb_account_uid), sqlc.narg(iban_last4),
        sqlc.narg(label), sqlc.arg(currency))
ON CONFLICT (connection_id, eb_account_uid)
DO UPDATE SET iban_last4 = EXCLUDED.iban_last4, label = EXCLUDED.label, currency = EXCLUDED.currency
RETURNING *;

-- name: ListAccounts :many
SELECT * FROM bank_accounts WHERE connection_id = $1 ORDER BY label;

-- name: GetTrackedAccount :one
SELECT * FROM bank_accounts WHERE connection_id = $1 AND is_tracked;

-- name: UntrackAccounts :exec
-- À appeler dans la même transaction que TrackAccount (index unique partiel).
UPDATE bank_accounts SET is_tracked = false WHERE connection_id = $1 AND is_tracked;

-- name: TrackAccount :exec
UPDATE bank_accounts SET is_tracked = true WHERE id = $1 AND connection_id = $2;
