-- name: StartSyncRun :one
INSERT INTO sync_runs (connection_id, trigger) VALUES ($1, $2) RETURNING id;

-- name: FinishSyncRun :exec
UPDATE sync_runs
SET status = sqlc.arg(status), finished_at = now(),
    fetched_count = sqlc.arg(fetched_count), inserted_count = sqlc.arg(inserted_count),
    error = sqlc.narg(error)
WHERE id = sqlc.arg(id);

-- name: CountManualSyncsToday :one
-- Limite des lectures non interactives (environ 4 par jour et par banque).
SELECT COUNT(*)::integer
FROM sync_runs
WHERE connection_id = $1
  AND started_at >= (date_trunc('day', now() AT TIME ZONE 'Europe/Paris') AT TIME ZONE 'Europe/Paris');

-- name: LastSyncRuns :many
SELECT * FROM sync_runs WHERE connection_id = $1 ORDER BY started_at DESC LIMIT 10;

-- name: InsertAudit :exec
INSERT INTO audit_log (user_id, action, detail) VALUES (sqlc.narg(user_id), sqlc.arg(action), sqlc.arg(detail));

-- name: PurgeAuditLog :execrows
-- Lignes conservées 12 mois.
DELETE FROM audit_log WHERE created_at < now() - interval '12 months';
