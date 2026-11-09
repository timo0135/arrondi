-- Statistiques de la version ordinateur (D-21) : tout est calculé ici, jamais dans le navigateur.

-- name: MonthStats :one
-- GET /stats/month : compteurs et sommes d'un mois (date de comptabilisation).
SELECT
    COUNT(*)::integer                                               AS transactions_count,
    COUNT(*) FILTER (WHERE t.is_eligible)::integer                  AS card_payments_count,
    COUNT(*) FILTER (WHERE r.final_cents > 0)::integer              AS rounded_count,
    COUNT(*) FILTER (WHERE NOT t.is_eligible)::integer              AS excluded_count,
    COUNT(*) FILTER (WHERE r.limited_by IS NOT NULL)::integer       AS capped_count,
    COALESCE(SUM(t.amount_cents) FILTER (WHERE t.is_eligible), 0)::bigint AS card_spending_cents,
    COALESCE(SUM(r.final_cents), 0)::bigint                         AS saved_cents
FROM transactions t
JOIN bank_accounts a    ON a.id = t.account_id
JOIN bank_connections c ON c.id = a.connection_id
LEFT JOIN roundings r   ON r.transaction_id = t.id
WHERE c.user_id = sqlc.arg(user_id)
  AND t.booked_on >= sqlc.arg(month)::date AND t.booked_on < (sqlc.arg(month)::date + interval '1 month');

-- name: MonthExclusions :many
SELECT t.exclusion_reason::text AS reason, COUNT(*)::integer AS count
FROM transactions t
JOIN bank_accounts a    ON a.id = t.account_id
JOIN bank_connections c ON c.id = a.connection_id
WHERE c.user_id = sqlc.arg(user_id) AND NOT t.is_eligible
  AND t.booked_on >= sqlc.arg(month)::date AND t.booked_on < (sqlc.arg(month)::date + interval '1 month')
GROUP BY t.exclusion_reason
ORDER BY 2 DESC, 1;

-- name: TopMerchants :many
-- « Où tu arrondis le plus » : les commerçants du mois au plus fort arrondi cumulé (D-23).
SELECT t.merchant, COUNT(*)::integer AS count, SUM(r.final_cents)::bigint AS saved_cents
FROM transactions t
JOIN roundings r ON r.transaction_id = t.id
WHERE r.user_id = sqlc.arg(user_id) AND r.month = sqlc.arg(month)::date AND r.final_cents > 0
GROUP BY t.merchant
ORDER BY 3 DESC, 2 DESC, 1
LIMIT sqlc.arg(page_size);

-- name: SavedBookedBetween :one
-- Épargne des transactions comptabilisées entre deux dates incluses.
SELECT COALESCE(SUM(r.final_cents), 0)::bigint
FROM roundings r JOIN transactions t ON t.id = r.transaction_id
WHERE r.user_id = sqlc.arg(user_id)
  AND t.booked_on >= sqlc.arg(from_day)::date AND t.booked_on <= sqlc.arg(to_day)::date;

-- name: SavedByMonthBetween :many
-- Épargne par mois sur [from_month, to_month[, pour le rythme de l'objectif (EF-41).
SELECT month, SUM(final_cents)::bigint AS saved_cents
FROM roundings
WHERE user_id = sqlc.arg(user_id) AND month >= sqlc.arg(from_month)::date AND month < sqlc.arg(to_month)::date
GROUP BY month
ORDER BY month;

-- name: EligibleAmountsInMonth :many
-- GET /settings/preview : montants éligibles d'un mois, dans l'ordre d'enregistrement de la synchro.
SELECT t.amount_cents
FROM transactions t
JOIN bank_accounts a    ON a.id = t.account_id
JOIN bank_connections c ON c.id = a.connection_id
WHERE c.user_id = sqlc.arg(user_id) AND t.is_eligible
  AND t.booked_on >= sqlc.arg(month)::date AND t.booked_on < (sqlc.arg(month)::date + interval '1 month')
ORDER BY t.booked_on, t.external_id;

-- name: ListMonthTransactions :many
-- GET /transactions/export : toutes les transactions d'un mois, de la plus ancienne à la plus récente.
SELECT t.booked_on, t.amount_cents, t.direction, t.label, t.merchant, t.exclusion_reason, r.final_cents
FROM transactions t
JOIN bank_accounts a    ON a.id = t.account_id
JOIN bank_connections c ON c.id = a.connection_id
LEFT JOIN roundings r   ON r.transaction_id = t.id
WHERE c.user_id = sqlc.arg(user_id)
  AND t.booked_on >= sqlc.arg(month)::date AND t.booked_on < (sqlc.arg(month)::date + interval '1 month')
ORDER BY t.booked_on, t.external_id;

-- name: ListSyncRuns :many
-- GET /bank/sync-runs (EF-43).
SELECT trigger, status, started_at, finished_at, fetched_count, inserted_count, error
FROM sync_runs
WHERE connection_id = sqlc.arg(connection_id)
ORDER BY started_at DESC
LIMIT sqlc.arg(page_size);
