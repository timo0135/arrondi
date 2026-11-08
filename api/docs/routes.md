# Référence des routes

Préfixe : `/api/v1`. JSON UTF-8. Le contrat complet (schémas, champs, contraintes) est généré depuis le code : `GET /api/v1/openapi.json`. Ce document en donne la lecture humaine.

## Conventions

- **Authentification** : toutes les routes exigent Cloudflare Access, sauf `GET /healthz` (servie à la racine, hors préfixe). En local, `APP_ENV=dev` remplace Cloudflare par une identité fixe.
- **Requêtes qui modifient des données** (`POST`, `PUT`, `PATCH`, `DELETE`) : en-tête `X-Requested-With: epargne` obligatoire, et `Content-Type: application/json` si elles ont un corps. Sinon : 400.
- **Montants** : entiers en centimes, champs terminés par `_cents`.
- **Dates** : `AAAA-MM-JJ` (calendrier, Europe/Paris) ; horodatages RFC 3339 en UTC ; mois `AAAA-MM`.
- **Pagination** : `limit` et `cursor` ; la réponse contient `items` et `next_cursor` (`null` sur la dernière page).
- **Erreurs** : `application/problem+json` (RFC 9457) avec `title`, `status`, `detail`.
- **Limites** : 60 requêtes par minute et par utilisateur, corps de 64 Ko au plus.

| Code | Signification |
| --- | --- |
| 400 | Requête mal formée (en-tête manquant, curseur ou état invalide) |
| 401 | Pas de JWT Cloudflare Access valide |
| 403 | Email inconnu, compte désactivé, ou route réservée à l'administrateur |
| 404 | Introuvable (ou pas à toi) |
| 409 | Conflit (autorisation déjà en cours, email existant, changement de banque…) |
| 422 | Valeur hors plage ou invalide |
| 429 | Trop de requêtes, ou quota de synchronisation atteint |
| 502 | La banque ou Enable Banking ne répond pas |

## Résumé

| Méthode et route | Rôle | Succès |
| --- | --- | --- |
| `GET /healthz` | aucun | 200 / 503 |
| `GET /me` | membre | 200 |
| `DELETE /me` | membre | 204 |
| `GET /me/export` | membre | 200 |
| `GET /dashboard` | membre | 200 |
| `GET /months` | membre | 200 |
| `GET /transactions` | membre | 200 |
| `GET /settings` · `PUT /settings` | membre | 200 |
| `GET /goal` · `PUT /goal` · `DELETE /goal` | membre | 200 / 204 |
| `GET /transfers` · `POST /transfers` · `DELETE /transfers/{id}` | membre | 200 / 201 / 204 |
| `GET /bank/connection` · `DELETE /bank/connection` | membre | 200 / 204 |
| `PUT /bank/credentials` | membre | 204 |
| `POST /bank/authorizations` | membre | 200 |
| `GET /bank/callback` | membre (navigateur) | 302 |
| `PUT /bank/account` | membre | 204 |
| `POST /bank/sync` | membre | 202 |
| `GET /admin/users` · `POST /admin/users` · `PATCH /admin/users/{id}` | administrateur | 200 / 201 |

---

## Compte

### `GET /me`

```json
{ "id": "fc69…", "email": "timothee@exemple.fr", "display_name": "Timothée", "role": "admin" }
```

### `DELETE /me`

Supprime le compte et **toutes** ses données (EF-30). Corps : `{ "confirm": true }`, sinon 422. Réponse 204.

### `GET /me/export`

Téléchargement JSON (EF-29) : utilisateur, réglages, objectif, virements et transactions avec leur arrondi (sans la réponse brute de la banque). En-tête `Content-Disposition: attachment`.

## Épargne

### `GET /dashboard`

```json
{
  "total_saved_cents": 18420,
  "month_saved_cents": 1235,
  "transferred_cents": 15000,
  "to_transfer_cents": 3420,
  "month": "2026-11",
  "goal": { "name": "Apport", "target_cents": 200000, "progress_percent": 9, "target_date": "2027-09-01" },
  "bank": { "status": "active", "expires_in_days": 112, "last_sync_ok_at": "2026-11-07T05:30:12Z" }
}
```

- `to_transfer_cents` = épargné − viré, jamais négatif.
- `progress_percent` = épargné / cible, plafonné à 100.
- `goal` vaut `null` sans objectif ; `bank` vaut `null` sans connexion.
- `bank.status` : `pending`, `active`, `expiring_soon` (consentement dans 14 jours ou moins), `expired`, `error`.

### `GET /months?limit=12`

`limit` de 1 à 24. Du plus récent au plus ancien.

```json
[ { "month": "2026-11", "saved_cents": 1235, "transferred_cents": 1500, "transactions_count": 18 } ]
```

### `GET /transactions?month=2026-11&limit=50&cursor=…`

`month` facultatif (`AAAA-MM`, sinon 422), `limit` de 1 à 100. De la plus récente à la plus ancienne.

```json
{
  "items": [
    {
      "id": "8b0c…", "booked_on": "2026-11-05", "amount_cents": 6937, "direction": "debit",
      "currency": "EUR", "label": "CARTE 04/11/26 TOTAL 4 CB*6804",
      "is_eligible": true, "exclusion_reason": null,
      "rounding": { "base_cents": 63, "gross_cents": 63, "final_cents": 63, "limited_by": null }
    },
    {
      "id": "f3a1…", "booked_on": "2026-11-05", "amount_cents": 10542, "direction": "debit",
      "currency": "EUR", "label": "PRLV SEPA MACIF", "is_eligible": false,
      "exclusion_reason": "not_card", "rounding": null
    }
  ],
  "next_cursor": "MjAyNi0xMS0wNXw4YjBj…"
}
```

`limited_by` : `null`, `tx_cap` (plafond par transaction) ou `monthly` (plafond mensuel). Motifs d'exclusion : voir [architecture](architecture.md#éligibilité-section-42).

## Réglages

### `GET /settings` · `PUT /settings`

```json
{ "step_cents": 100, "multiplier": 1, "tx_cap_cents": 2000, "month_cap_cents": null }
```

| Champ | Valeurs permises |
| --- | --- |
| `step_cents` | 50 à 5000, multiple de 50 |
| `multiplier` | 1, 2, 3, 5, 10 |
| `tx_cap_cents` | 100 à 10000, multiple de 100 |
| `month_cap_cents` | `null` (aucun plafond) ou 500 à 100000, multiple de 500 |

Hors plage : 422. Pas d'effet rétroactif (D-08).

## Objectif

`GET /goal` (404 s'il n'y en a pas) · `PUT /goal` · `DELETE /goal` (204).

```json
{ "name": "Apport", "target_cents": 200000, "target_date": "2027-09-01" }
```

`target_cents` > 0, `name` de 1 à 60 caractères, `target_date` facultative.

## Virements

### `GET /transfers?limit=50&cursor=…`

Page de virements, du plus récent au plus ancien (même forme `items` / `next_cursor`).

### `POST /transfers`

```json
{ "amount_cents": 1500, "transferred_on": "2026-11-08" }
```

201 ; 422 si le montant n'est pas strictement positif ou la date est dans le futur.

### `DELETE /transfers/{id}`

204 ; 404 si le virement n'existe pas ou n'est pas le tien.

## Banque

Parcours complet : `PUT /bank/credentials` → `POST /bank/authorizations` → validation chez la banque dans le navigateur → `GET /bank/callback` → (`PUT /bank/account`) → `POST /bank/sync`.

### `GET /bank/connection`

```json
{
  "bank_code": "boursobank", "aspsp_name": "Boursorama Banque", "status": "active",
  "consent_valid_until": "2027-05-07T15:00:00Z", "expires_in_days": 180,
  "activated_at": "2026-11-08T15:00:00Z", "last_sync_at": "…", "last_sync_ok_at": "…", "last_error": null,
  "accounts": [
    { "id": "1c2d…", "label": "CAV - BOURSOBANK", "iban_last4": "7514", "currency": "EUR", "is_tracked": true },
    { "id": "9e8f…", "label": "Carte bancaire à débit immédiat", "iban_last4": null, "currency": "EUR", "is_tracked": false }
  ]
}
```

404 sans connexion.

### `PUT /bank/credentials`

```json
{
  "bank_code": "boursobank",
  "eb_application_id": "a0def487-…",
  "private_key_pem": "-----BEGIN PRIVATE KEY-----\n…\n-----END PRIVATE KEY-----\n"
}
```

- `aspsp_name` : facultatif pour une banque à un seul nom (BoursoBank), obligatoire pour une banque à plusieurs caisses.
- Crée la connexion (`pending`) ou remplace les identifiants. La clé est chiffrée et n'est jamais renvoyée.
- 204 ; 422 si la banque n'est pas prise en charge ou si la clé n'est pas une clé RSA valide ; 409 si une connexion existe déjà pour une autre banque.

### `POST /bank/authorizations`

Démarre l'autorisation (section 9.2) : état aléatoire de 32 octets valable 10 minutes, durée de consentement = maximum de la banque.

```json
{ "url": "https://tilisy.enablebanking.com/ais/start?sessionid=…" }
```

L'application redirige l'utilisateur vers cette adresse. 404 sans identifiants enregistrés, 409 si une autorisation est déjà en cours, 422 si Enable Banking refuse l'identifiant ou la clé, 502 si la banque ne répond pas.

### `GET /bank/callback?code=…&state=…`

Appelée par le navigateur au retour de la banque (URL déclarée dans la console Enable Banking). L'état doit appartenir à l'utilisateur, ne pas avoir expiré et ne servir qu'une fois. Crée la session, enregistre les comptes et, s'il n'y a qu'un compte synchronisable, le suit automatiquement.

Réponse : 302 vers `PUBLIC_BASE_URL/reglages/banque?status=ok`, `status=refused` (refus chez la banque) ou `status=error` (échec de la création de session). 400 si l'état est inconnu, expiré ou déjà utilisé.

### `PUT /bank/account`

```json
{ "account_id": "1c2d…" }
```

Désigne le compte suivi. 204 ; 404 si le compte n'est pas le tien.

### `POST /bank/sync`

Lance une synchronisation en arrière-plan. 202 `{ "status": "accepted" }` ; 409 si la connexion n'est pas active ou sans compte suivi ; 429 au-delà de 1 par heure ou 4 sur 24 heures (EF-10). Relire `GET /dashboard` ou `GET /bank/connection` quelques secondes après.

### `DELETE /bank/connection`

Supprime la connexion, ses comptes, transactions et arrondis (EF-08). 204.

## Administration

Réservé au rôle `admin` (403 sinon).

| Route | Corps | Réponse |
| --- | --- | --- |
| `GET /admin/users` | – | Liste : `id`, `email`, `display_name`, `role`, `is_active`, `connection_status` |
| `POST /admin/users` | `{ "email": "…", "display_name": "…" }` | 201 ; rôle toujours `member` ; réglages par défaut créés ; 409 si l'email existe |
| `PATCH /admin/users/{id}` | `{ "is_active": false }` | 200 ; 409 si l'administrateur se désactive lui-même |

Inviter quelqu'un dans l'API ne suffit pas : son email doit aussi être ajouté à la liste autorisée de Cloudflare Access.
