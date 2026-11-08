-- name: SavingsTotals :one
-- Totaux du tableau de bord : épargné (somme des arrondis), viré, et du mois en cours.
SELECT
    (SELECT COALESCE(SUM(final_cents), 0) FROM roundings r WHERE r.user_id = sqlc.arg(user_id))::bigint
        AS total_saved_cents,
    (SELECT COALESCE(SUM(final_cents), 0) FROM roundings r
      WHERE r.user_id = sqlc.arg(user_id) AND r.month = sqlc.arg(month))::bigint
        AS month_saved_cents,
    (SELECT COALESCE(SUM(amount_cents), 0) FROM transfers t WHERE t.user_id = sqlc.arg(user_id))::bigint
        AS total_transferred_cents;

-- name: MonthlySavings :many
-- Épargne par mois, pour l'historique et le graphique.
SELECT month, SUM(final_cents)::bigint AS saved_cents, COUNT(*)::integer AS rounding_count
FROM roundings
WHERE user_id = $1
GROUP BY month
ORDER BY month DESC
LIMIT 24;

-- name: CreateTransfer :one
INSERT INTO transfers (user_id, amount_cents, transferred_on)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListTransfers :many
SELECT * FROM transfers WHERE user_id = $1 ORDER BY transferred_on DESC, created_at DESC;

-- name: DeleteTransfer :execrows
DELETE FROM transfers WHERE id = $1 AND user_id = $2;

-- name: GetGoal :one
SELECT * FROM goals WHERE user_id = $1;

-- name: UpsertGoal :one
INSERT INTO goals (user_id, name, target_cents, target_date)
VALUES (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(target_cents), sqlc.narg(target_date))
ON CONFLICT (user_id)
DO UPDATE SET name = EXCLUDED.name, target_cents = EXCLUDED.target_cents,
              target_date = EXCLUDED.target_date, updated_at = now()
RETURNING *;

-- name: DeleteGoal :exec
DELETE FROM goals WHERE user_id = $1;
