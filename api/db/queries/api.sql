-- Requêtes propres aux routes HTTP (section 7).

-- name: CountUsers :one
SELECT COUNT(*)::integer FROM users;

-- name: UpsertCredentials :one
-- PUT /bank/credentials : crée la connexion (pending) ou remplace les identifiants.
-- La banque d'une connexion existante ne change pas ici (contrôlé par l'API).
INSERT INTO bank_connections (user_id, bank_code, eb_application_id, eb_private_key_enc, eb_key_nonce, aspsp_name)
VALUES (sqlc.arg(user_id), sqlc.arg(bank_code), sqlc.arg(eb_application_id), sqlc.arg(eb_private_key_enc),
        sqlc.arg(eb_key_nonce), sqlc.arg(aspsp_name))
ON CONFLICT (user_id) DO UPDATE
SET eb_application_id  = EXCLUDED.eb_application_id,
    eb_private_key_enc = EXCLUDED.eb_private_key_enc,
    eb_key_nonce       = EXCLUDED.eb_key_nonce,
    updated_at         = now()
RETURNING *;

-- name: BeginAuthorization :execrows
-- Réserve l'état aléatoire ; échoue (0 ligne) si une autorisation non expirée est déjà en cours.
UPDATE bank_connections
SET auth_state = sqlc.arg(auth_state), auth_state_expires_at = sqlc.arg(auth_state_expires_at), updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND (auth_state IS NULL OR auth_state_expires_at <= now());

-- name: CancelAuthorization :exec
UPDATE bank_connections SET auth_state = NULL, auth_state_expires_at = NULL, updated_at = now()
WHERE user_id = $1;

-- name: ConsumeAuthState :one
-- Callback : l'état doit appartenir à l'utilisateur et ne pas être expiré ; il est effacé (usage unique).
UPDATE bank_connections
SET auth_state = NULL, auth_state_expires_at = NULL, updated_at = now()
WHERE user_id = sqlc.arg(user_id) AND auth_state = sqlc.arg(auth_state) AND auth_state_expires_at > now()
RETURNING *;

-- name: DeleteConnection :execrows
-- EF-08 : supprime la connexion, ses comptes, transactions et arrondis (cascade).
DELETE FROM bank_connections WHERE user_id = $1;

-- name: GetAccountForUser :one
SELECT a.* FROM bank_accounts a
JOIN bank_connections c ON c.id = a.connection_id
WHERE a.id = sqlc.arg(account_id) AND c.user_id = sqlc.arg(user_id);

-- name: ListTransactionsPage :many
-- GET /transactions : filtres facultatifs (mois, arrondies ou exclues, texte du libellé),
-- pagination par curseur (booked_on, id).
SELECT t.id, t.booked_on, t.amount_cents, t.direction, t.currency, t.label, t.merchant,
       t.is_eligible, t.exclusion_reason,
       r.step_cents, r.multiplier, r.base_cents, r.gross_cents, r.final_cents, r.limited_by
FROM transactions t
JOIN bank_accounts a    ON a.id = t.account_id
JOIN bank_connections c ON c.id = a.connection_id
LEFT JOIN roundings r   ON r.transaction_id = t.id
WHERE c.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(month)::date IS NULL
       OR (t.booked_on >= sqlc.narg(month)::date AND t.booked_on < (sqlc.narg(month)::date + interval '1 month')))
  AND (sqlc.narg(status)::text IS NULL
       OR (sqlc.narg(status)::text = 'rounded' AND t.is_eligible)
       OR (sqlc.narg(status)::text = 'excluded' AND NOT t.is_eligible))
  -- q arrive déjà échappé pour LIKE (\, %, _) et sans accents ni majuscules.
  AND (sqlc.narg(q)::text IS NULL
       OR unaccent(lower(t.label)) LIKE '%' || sqlc.narg(q)::text || '%')
  AND (sqlc.narg(before_booked_on)::date IS NULL
       OR (t.booked_on, t.id) < (sqlc.narg(before_booked_on)::date, sqlc.narg(before_id)::uuid))
ORDER BY t.booked_on DESC, t.id DESC
LIMIT sqlc.arg(page_size);

-- name: GetTransactionForUser :one
-- GET /transactions/{id} : 404 si la transaction n'appartient pas à l'utilisateur.
SELECT t.id, t.booked_on, t.amount_cents, t.direction, t.currency, t.label, t.merchant,
       t.is_eligible, t.exclusion_reason,
       r.step_cents, r.multiplier, r.base_cents, r.gross_cents, r.final_cents, r.limited_by
FROM transactions t
JOIN bank_accounts a    ON a.id = t.account_id
JOIN bank_connections c ON c.id = a.connection_id
LEFT JOIN roundings r   ON r.transaction_id = t.id
WHERE c.user_id = sqlc.arg(user_id) AND t.id = sqlc.arg(id);

-- name: MonthSummaries :many
-- GET /months : épargne, virements et nombre de transactions arrondies par mois.
WITH s AS (
    SELECT month, SUM(final_cents)::bigint AS saved_cents, COUNT(*)::integer AS transactions_count
    FROM roundings ro WHERE ro.user_id = sqlc.arg(user_id) GROUP BY month
), v AS (
    SELECT date_trunc('month', transferred_on)::date AS month, SUM(amount_cents)::bigint AS transferred_cents
    FROM transfers tr WHERE tr.user_id = sqlc.arg(user_id) GROUP BY 1
)
SELECT COALESCE(s.month, v.month)::date            AS month,
       COALESCE(s.saved_cents, 0)::bigint           AS saved_cents,
       COALESCE(v.transferred_cents, 0)::bigint     AS transferred_cents,
       COALESCE(s.transactions_count, 0)::integer   AS transactions_count
FROM s FULL JOIN v ON v.month = s.month
ORDER BY 1 DESC
LIMIT sqlc.arg(page_size);

-- name: ListTransfersPage :many
SELECT * FROM transfers
WHERE user_id = sqlc.arg(user_id)
  AND (sqlc.narg(before_on)::date IS NULL
       OR (transferred_on, id) < (sqlc.narg(before_on)::date, sqlc.narg(before_id)::uuid))
ORDER BY transferred_on DESC, id DESC
LIMIT sqlc.arg(page_size);

-- name: CountSyncRunsSince :one
-- EF-10 : au plus 1 synchro par heure et 4 sur 24 heures.
SELECT COUNT(*)::integer FROM sync_runs
WHERE connection_id = sqlc.arg(connection_id) AND started_at > sqlc.arg(since);

-- name: PurgeOldTransactions :execrows
-- D-10 : transactions (et leurs arrondis) conservées 24 mois.
DELETE FROM transactions WHERE booked_on < (current_date - interval '24 months');

-- name: ListUsersWithConnection :many
SELECT u.id, u.email, u.display_name, u.role, u.is_active, c.status AS connection_status,
       c.consent_valid_until, c.last_sync_ok_at
FROM users u LEFT JOIN bank_connections c ON c.user_id = u.id
ORDER BY u.created_at;

-- name: ExportTransactions :many
-- EF-29 : export complet (sans le JSON brut de la banque).
SELECT t.booked_on, t.amount_cents, t.direction, t.currency, t.label, t.is_eligible, t.exclusion_reason,
       r.final_cents, r.month
FROM transactions t
JOIN bank_accounts a    ON a.id = t.account_id
JOIN bank_connections c ON c.id = a.connection_id
LEFT JOIN roundings r   ON r.transaction_id = t.id
WHERE c.user_id = $1
ORDER BY t.booked_on, t.id;
