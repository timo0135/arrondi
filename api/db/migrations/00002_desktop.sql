-- Version ordinateur (D-20 à D-23) : nom du commerçant et recherche dans les libellés.

-- +goose Up
-- unaccent est une extension « trusted » : le propriétaire de la base peut la créer.
CREATE EXTENSION IF NOT EXISTS unaccent;

ALTER TABLE transactions ADD COLUMN merchant text;

-- Transactions déjà importées (toutes BoursoBank à ce jour) : même règle que
-- bank.BoursoBank.Merchant puis bank.Initcap, écrite en SQL.
UPDATE transactions
SET merchant = initcap(btrim(
        regexp_replace(
            regexp_replace(
                regexp_replace(btrim(label), '^CARTE [0-9]{2}/[0-9]{2}(/[0-9]{2})? +', ''),
                ' +CB\*[0-9]+$', ''),
            '^PRLV SEPA +', '')));

ALTER TABLE transactions ALTER COLUMN merchant SET NOT NULL;

-- +goose Down
ALTER TABLE transactions DROP COLUMN merchant;
