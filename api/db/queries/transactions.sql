-- name: LockUserForSync :exec
-- Verrou de transaction par utilisateur : deux synchros (planifiée et manuelle)
-- ne calculent jamais le cumul mensuel en même temps.
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(user_id)::uuid::text, 0));

-- name: InsertTransaction :one
-- Idempotent : une transaction déjà connue n'est pas réinsérée (aucune ligne renvoyée).
INSERT INTO transactions (account_id, external_id, booked_on, amount_cents, direction, currency,
                          label, bank_tx_code, is_eligible, exclusion_reason, raw)
VALUES (sqlc.arg(account_id), sqlc.arg(external_id), sqlc.arg(booked_on), sqlc.arg(amount_cents),
        sqlc.arg(direction), sqlc.arg(currency), sqlc.arg(label), sqlc.narg(bank_tx_code),
        sqlc.arg(is_eligible), sqlc.narg(exclusion_reason), sqlc.arg(raw))
ON CONFLICT (account_id, external_id) DO NOTHING
RETURNING id;

-- name: MonthToDateCents :one
-- Somme des arrondis finaux déjà enregistrés pour un mois : entrée du moteur (R_mois).
SELECT COALESCE(SUM(final_cents), 0)::bigint AS total
FROM roundings
WHERE user_id = sqlc.arg(user_id) AND month = sqlc.arg(month);

-- name: InsertRounding :exec
INSERT INTO roundings (transaction_id, user_id, step_cents, multiplier, base_cents, gross_cents,
                       final_cents, limited_by, month)
VALUES (sqlc.arg(transaction_id), sqlc.arg(user_id), sqlc.arg(step_cents), sqlc.arg(multiplier),
        sqlc.arg(base_cents), sqlc.arg(gross_cents), sqlc.arg(final_cents), sqlc.narg(limited_by),
        sqlc.arg(month));

-- name: ListTransactions :many
-- Historique de l'utilisateur, le plus récent d'abord, pagination par curseur (booked_on, id).
SELECT t.id, t.booked_on, t.amount_cents, t.direction, t.currency, t.label,
       t.is_eligible, t.exclusion_reason,
       r.base_cents, r.gross_cents, r.final_cents, r.limited_by
FROM transactions t
JOIN bank_accounts a     ON a.id = t.account_id
JOIN bank_connections c  ON c.id = a.connection_id
LEFT JOIN roundings r    ON r.transaction_id = t.id
WHERE c.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(before_booked_on)::date IS NULL
       OR (t.booked_on, t.id) < (sqlc.narg(before_booked_on)::date, sqlc.narg(before_id)::uuid))
ORDER BY t.booked_on DESC, t.id DESC
LIMIT sqlc.arg(page_size);

-- name: GetTransaction :one
-- Lecture d'une transaction en vérifiant qu'elle appartient à l'utilisateur.
SELECT t.*
FROM transactions t
JOIN bank_accounts a    ON a.id = t.account_id
JOIN bank_connections c ON c.id = a.connection_id
WHERE t.id = sqlc.arg(id) AND c.user_id = sqlc.arg(user_id);

-- name: LastBookedOn :one
-- Date de la dernière transaction connue d'un compte (fenêtre de relecture de 7 jours).
SELECT booked_on FROM transactions WHERE account_id = $1 ORDER BY booked_on DESC LIMIT 1;
