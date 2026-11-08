-- Schéma initial d'Arrondi (cahier des charges, section 6).
-- Montants en centimes (bigint), jamais en numeric ni en float.
-- Les clés étrangères vers users se suppriment en cascade (EF-30), sauf audit_log.

-- +goose Up
-- +goose StatementBegin

CREATE TABLE users (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    email        text        NOT NULL UNIQUE CHECK (email = lower(email)),
    display_name text        NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 60),
    role         text        NOT NULL CHECK (role IN ('admin', 'member')),
    is_active    boolean     NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_settings (
    user_id         uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    step_cents      integer     NOT NULL DEFAULT 100
                    CHECK (step_cents BETWEEN 50 AND 5000 AND step_cents % 50 = 0),
    multiplier      integer     NOT NULL DEFAULT 1 CHECK (multiplier IN (1, 2, 3, 5, 10)),
    tx_cap_cents    integer     NOT NULL DEFAULT 2000
                    CHECK (tx_cap_cents BETWEEN 100 AND 10000 AND tx_cap_cents % 100 = 0),
    month_cap_cents integer     -- NULL = aucun plafond mensuel
                    CHECK (month_cap_cents IS NULL
                           OR (month_cap_cents BETWEEN 500 AND 100000 AND month_cap_cents % 500 = 0)),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE bank_connections (
    id                    uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               uuid        NOT NULL UNIQUE REFERENCES users (id) ON DELETE CASCADE,
    bank_code             text        NOT NULL,   -- code du profil de banque ("boursobank")
    eb_application_id     text        NOT NULL,
    eb_private_key_enc    bytea       NOT NULL,   -- clé privée chiffrée (AES-256-GCM, section 10)
    eb_key_nonce          bytea       NOT NULL CHECK (octet_length(eb_key_nonce) = 12),
    eb_session_id         text,
    aspsp_name            text        NOT NULL,
    aspsp_country         text        NOT NULL DEFAULT 'FR' CHECK (aspsp_country = 'FR'),
    consent_valid_until   timestamptz,
    activated_at          timestamptz,           -- fixée au premier consentement réussi, jamais modifiée
    status                text        NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending', 'active', 'expired', 'error')),
    auth_state            text,
    auth_state_expires_at timestamptz,
    last_sync_at          timestamptz,
    last_sync_ok_at       timestamptz,
    last_error            text,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CHECK ((auth_state IS NULL) = (auth_state_expires_at IS NULL)),
    CHECK (status <> 'active' OR (eb_session_id IS NOT NULL AND consent_valid_until IS NOT NULL
                                  AND activated_at IS NOT NULL))
);

CREATE UNIQUE INDEX bank_connections_auth_state_key ON bank_connections (auth_state)
    WHERE auth_state IS NOT NULL;

CREATE TABLE bank_accounts (
    id             uuid    PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id  uuid    NOT NULL REFERENCES bank_connections (id) ON DELETE CASCADE,
    eb_account_uid text    NOT NULL,
    iban_last4     char(4),                    -- l'IBAN complet n'est jamais stocké
    label          text,
    currency       char(3) NOT NULL,
    is_tracked     boolean NOT NULL DEFAULT false,
    UNIQUE (connection_id, eb_account_uid)
);

-- Un seul compte suivi par connexion, donc par utilisateur.
CREATE UNIQUE INDEX bank_accounts_one_tracked ON bank_accounts (connection_id) WHERE is_tracked;

CREATE TABLE transactions (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id       uuid        NOT NULL REFERENCES bank_accounts (id) ON DELETE CASCADE,
    external_id      text        NOT NULL,
    booked_on        date        NOT NULL,
    amount_cents     bigint      NOT NULL CHECK (amount_cents >= 0),
    direction        text        NOT NULL CHECK (direction IN ('debit', 'credit')),
    currency         char(3)     NOT NULL,
    label            text        NOT NULL,
    bank_tx_code     text,
    is_eligible      boolean     NOT NULL,
    exclusion_reason text CHECK (exclusion_reason IN ('credit', 'foreign_currency', 'zero_amount',
                                 'before_activation', 'not_card', 'cash_withdrawal', 'untracked_account')),
    raw              jsonb       NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account_id, external_id),
    CHECK (is_eligible = (exclusion_reason IS NULL))
);

CREATE INDEX transactions_account_booked ON transactions (account_id, booked_on DESC);

-- Une ligne par transaction éligible, jamais modifiée.
CREATE TABLE roundings (
    transaction_id uuid        PRIMARY KEY REFERENCES transactions (id) ON DELETE CASCADE,
    user_id        uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    step_cents     integer     NOT NULL,
    multiplier     integer     NOT NULL,
    base_cents     integer     NOT NULL CHECK (base_cents >= 0),
    gross_cents    bigint      NOT NULL CHECK (gross_cents >= 0),
    final_cents    bigint      NOT NULL CHECK (final_cents >= 0 AND final_cents <= gross_cents),
    limited_by     text        CHECK (limited_by IN ('tx_cap', 'monthly')),
    month          date        NOT NULL CHECK (extract(day FROM month) = 1),
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX roundings_user_month ON roundings (user_id, month);

CREATE TABLE transfers (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    amount_cents   bigint      NOT NULL CHECK (amount_cents > 0),
    transferred_on date        NOT NULL,      -- la non-futurité est contrôlée par l'API
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX transfers_user ON transfers (user_id, transferred_on DESC);

CREATE TABLE goals (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid        NOT NULL UNIQUE REFERENCES users (id) ON DELETE CASCADE,
    name         text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    target_cents bigint      NOT NULL CHECK (target_cents > 0),
    target_date  date,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sync_runs (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id  uuid        NOT NULL REFERENCES bank_connections (id) ON DELETE CASCADE,
    trigger        text        NOT NULL CHECK (trigger IN ('scheduled', 'manual')),
    status         text        NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'error')),
    started_at     timestamptz NOT NULL DEFAULT now(),
    finished_at    timestamptz,
    fetched_count  integer     NOT NULL DEFAULT 0,
    inserted_count integer     NOT NULL DEFAULT 0,
    error          text,
    CHECK ((status = 'running') = (finished_at IS NULL))
);

CREATE INDEX sync_runs_connection ON sync_runs (connection_id, started_at DESC);

CREATE TABLE audit_log (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    uuid        REFERENCES users (id) ON DELETE SET NULL,
    action     text        NOT NULL,
    detail     jsonb       NOT NULL DEFAULT '{}',   -- ni email, ni libellé, ni montant
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_created ON audit_log (created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE audit_log, sync_runs, goals, transfers, roundings, transactions,
           bank_accounts, bank_connections, user_settings, users;
-- +goose StatementEnd
