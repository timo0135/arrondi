# Arrondi – API

API REST en Go de l'application Arrondi : comptes, réglages d'arrondi, connexion bancaire Enable Banking, synchronisation quotidienne des transactions et calcul des arrondis.

- **Langage** : Go 1.24, `net/http` + [huma](https://huma.rocks) (contrat OpenAPI généré)
- **Base** : PostgreSQL 18, accès par [pgx](https://github.com/jackc/pgx) + [sqlc](https://sqlc.dev), migrations [goose](https://github.com/pressly/goose)
- **Authentification** : Cloudflare Access (aucune authentification maison)
- **Banque** : Enable Banking, lecture seule, une application par utilisateur

Sommaire : [Démarrage](#démarrage) · [Commandes](#commandes) · [Configuration](#configuration) · [Tests](#tests) · [Organisation du code](#organisation-du-code) · [Postman](#postman) · [Dépannage](#dépannage)

Documents détaillés : [architecture](docs/architecture.md) · [routes](docs/routes.md) · [exploitation](docs/exploitation.md)

---

## Démarrage

Prérequis : **Go 1.24+**, **Docker Desktop** lancé, **make**.

```sh
cd api
go mod tidy                                  # télécharge les dépendances, complète go.sum
cp .env.example .env                         # configuration locale (ignorée par Git)
openssl rand -base64 32 > master_key.dev     # clé maître de dev (ignorée par Git)
make db-up                                   # PostgreSQL 18 dans Docker (127.0.0.1:5432)
make migrate                                 # crée les tables
make run                                     # API sur http://localhost:8080
```

Vérification :

```sh
curl localhost:8080/healthz          # {"status":"ok"}
curl localhost:8080/api/v1/me        # ton compte administrateur
```

En mode dev (`APP_ENV=dev`), Cloudflare Access est remplacé par l'email de `DEV_USER_EMAIL`, et `BOOTSTRAP_ADMIN_EMAIL` crée l'administrateur au premier lancement si la base est vide.

Le contrat OpenAPI est servi sur `http://localhost:8080/api/v1/openapi.json` (importable dans Postman ou dans un éditeur Swagger). La page `/api/v1/docs` de huma reste vide : elle charge des scripts externes que la politique de sécurité `default-src 'self'` bloque volontairement.

## Commandes

Toutes depuis `api/`.

| Commande | Effet |
| --- | --- |
| `make db-up` | Démarre PostgreSQL 18 (`docker-compose.dev.yml`), attend qu'il soit prêt |
| `make db-down` | Arrête PostgreSQL ; les données restent dans le volume `arrondi-db-dev` |
| `make migrate` | Applique les migrations (`db/migrations`) |
| `make migrate-down` | Annule la dernière migration |
| `make migrate-status` | État des migrations |
| `make sqlc` | Régénère `internal/store` depuis `db/queries` (sqlc dans Docker, rien à installer) |
| `make run` | Charge `.env` et lance l'API |
| `make test` | Tests unitaires (les tests d'intégration sont ignorés sans base) |

Pour repartir d'une base vide : `make db-down && docker volume rm arrondi-db-dev && make db-up && make migrate`.

## Configuration

Variables d'environnement lues au démarrage (`cmd/api/main.go`).

| Variable | Obligatoire | Défaut | Rôle |
| --- | --- | --- | --- |
| `DATABASE_URL` | oui | – | Adresse PostgreSQL (secret en production) |
| `PUBLIC_BASE_URL` | oui | – | `https://arrondi.timotheebenchergui.fr` ; sert à l'URL de retour de la banque et aux redirections |
| `MASTER_KEY_FILE` | non | `/run/secrets/master_key` | Clé maître de 32 octets (brute, hexadécimale ou base64) qui chiffre les clés privées Enable Banking |
| `CF_ACCESS_TEAM_DOMAIN` | en production | – | `<équipe>.cloudflareaccess.com` |
| `CF_ACCESS_AUD` | en production | – | Audience de l'application Cloudflare Access |
| `SYNC_CRON` | non | `CRON_TZ=Europe/Paris 30 6 * * *` | Heure de la synchronisation quotidienne |
| `LISTEN_ADDR` | non | `:8080` | Adresse d'écoute |
| `LOG_LEVEL` | non | `info` | `debug`, `info`, `warn`, `error` |
| `BOOTSTRAP_ADMIN_EMAIL` | non | – | Crée cet administrateur si la base ne contient aucun utilisateur |
| `EB_BASE_URL` | non | `https://api.enablebanking.com` | Adresse de l'API Enable Banking |
| `APP_ENV` | dev | – | `dev` active l'identité fixe ci-dessous |
| `DEV_USER_EMAIL` | dev | – | Email utilisé à la place de Cloudflare Access. **Jamais en production.** |

> ⚠️ La clé maître ne doit jamais être perdue ni changée : sans elle, les clés privées Enable Banking stockées en base sont illisibles et chaque utilisateur doit les ressaisir. Garde-la dans un gestionnaire de mots de passe.

## Tests

```sh
make test                      # unitaires
TEST_DATABASE_URL="postgres://arrondi:arrondi-dev@127.0.0.1:5432/arrondi?sslmode=disable" go test ./...
```

Avec `TEST_DATABASE_URL`, deux tests d'intégration s'ajoutent (base migrée requise) :

- `internal/store` : synchronisation complète en base, deux passages sans doublon.
- `internal/httpapi` : parcours HTTP complet avec une fausse banque (identifiants, autorisation, retour de la banque, synchronisation, tableau de bord, pagination, virements, objectif, administration, export, suppression).

Chaque test crée ses propres utilisateurs et les supprime à la fin.

| Package | Ce qui est testé |
| --- | --- |
| `rounding` | Les 12 exemples de référence du cahier des charges (section 4.4), transactions réelles du J1, plafond mensuel, validation des réglages |
| `bank` | Normalisation des transactions BoursoBank réelles, conversion des montants sans float, règle `CARTE ` |
| `syncer` | Fenêtre de lecture, idempotence, éligibilité, plafond sur plusieurs transactions, erreurs bancaires, annulation du lot |
| `enablebanking` | Pagination, paramètres, classement des erreurs HTTP |
| `auth` | JWT valide, expiré, mauvaise audience, mauvais émetteur, fausse signature, rotation des clés |
| `keybox` | Chiffrement, refus d'un chiffré altéré ou d'un autre utilisateur, formats de clé maître |

## Organisation du code

```
api/
├── cmd/api/              binaire : configuration, serveur HTTP, cron 06:30
├── db/
│   ├── migrations/       schéma (goose)
│   └── queries/          requêtes SQL écrites à la main (sqlc)
├── internal/
│   ├── rounding/         moteur d'arrondis, pur (aucune dépendance)
│   ├── bank/             interface Profile, normalisation, profil BoursoBank
│   ├── enablebanking/    client HTTP Enable Banking
│   ├── syncer/           synchronisation et éligibilité (sans SQL ni HTTP)
│   ├── store/            code généré par sqlc + adaptateur pour syncer
│   ├── keybox/           chiffrement AES-256-GCM des clés privées
│   ├── auth/             vérification du JWT Cloudflare Access
│   ├── app/              logique partagée API / planificateur
│   └── httpapi/          routes, middlewares de sécurité
├── postman/              collection et environnements Postman
├── docs/                 documentation détaillée
├── sqlc.yaml
└── Makefile
```

Règles du code :

- Montants **toujours en centimes** (`int64`), jamais en float, y compris à la lecture des chaînes renvoyées par la banque.
- Dates de calendrier en Europe/Paris ; horodatages stockés en UTC.
- `rounding` et `syncer` ne connaissent ni SQL ni HTTP : on les teste sans rien démarrer.
- Les requêtes SQL s'écrivent dans `db/queries/*.sql`, puis `make sqlc`. On ne modifie jamais les fichiers `*.sql.go`, `db.go` et `models.go` à la main.
- Une nouvelle banque = un fichier dans `internal/bank/` qui implémente `Profile`, ajouté à la liste dans `internal/app/app.go`, après validation de sa fiche de mapping sur un compte réel.

## Postman

Importer les trois fichiers de [`postman/`](postman/) (collection + environnements `local` et `production`).

- **Local** : rien à configurer, l'API te connecte d'office.
- **Production** : `cloudflared access login https://arrondi.timotheebenchergui.fr`, puis `cloudflared access token -app=https://arrondi.timotheebenchergui.fr`, et colle le jeton dans `cf_access_token`.
- Les identifiants renvoyés (compte, virement, utilisateur, curseurs) et l'en-tête `X-Requested-With` sont gérés automatiquement.

## Dépannage

| Symptôme | Cause probable |
| --- | --- |
| `missing go.sum entry` | `go mod tidy` pas lancé dans `api/` |
| `base de données injoignable` | Docker Desktop arrêté, ou `make db-up` pas lancé |
| `clé maître : …` | `master_key.dev` absent ou mal formé : `openssl rand -base64 32 > master_key.dev` |
| `401` sur toutes les routes | Production sans jeton Cloudflare Access, ou `APP_ENV=dev` absent en local |
| `403 compte inconnu ou désactivé` | L'email n'existe pas en base : l'inviter, ou vérifier `BOOTSTRAP_ADMIN_EMAIL` |
| `400 en-tête X-Requested-With manquant` | Requête de modification sans `X-Requested-With: epargne` |
| `409` sur `POST /bank/authorizations` | Une autorisation est déjà en cours : attendre 10 minutes |
| `429` sur `POST /bank/sync` | 1 synchronisation par heure, 4 sur 24 heures |
| `connexion error` dans `GET /bank/connection` | Voir `last_error` ; détail des cas dans [exploitation](docs/exploitation.md#erreurs-bancaires) |
