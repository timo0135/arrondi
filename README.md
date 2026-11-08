# Arrondi

> Ce que tu arrondis, tu le gardes.

Application privée d'épargne par arrondis : chaque paiement par carte est arrondi (à 1, 2, 5 ou 10 €, ou un palier personnalisé) et la différence est comptée comme épargne. L'application lit les transactions bancaires en lecture seule via [Enable Banking](https://enablebanking.com) ; le virement vers l'épargne reste manuel.

Usage strictement personnel et familial (10 utilisateurs au plus), hébergé sur un Mac mini, exposé uniquement par Cloudflare Tunnel et Cloudflare Access.

## Contenu du dépôt

| Dossier | Rôle |
| --- | --- |
| [`api/`](api/README.md) | API REST en Go : moteur d'arrondis, synchronisation bancaire, PostgreSQL |
| `web/` | PWA Vue 3 (à venir) |
| `api-j1/` | Preuve de concept du jalon J1 (lecture des transactions BoursoBank), conservée pour référence |
| `Documents/` | Cahier des charges, étude de marché, charte graphique et maquettes (PDF v1) |
| `docker-compose.dev.yml` | PostgreSQL 18 pour le développement local |

## Démarrage rapide (API)

Prérequis : Go 1.24+, Docker Desktop, `make`.

```sh
cd api
go mod tidy
cp .env.example .env
openssl rand -base64 32 > master_key.dev
make db-up && make migrate
make run
```

Puis `curl localhost:8080/api/v1/me`. Détails, configuration et tests : [`api/README.md`](api/README.md).

## Documentation

- [API : installation, commandes, configuration, tests](api/README.md)
- [Architecture et modèle de données](api/docs/architecture.md)
- [Référence des routes](api/docs/routes.md)
- [Exploitation : sécurité, synchronisation, erreurs, déploiement](api/docs/exploitation.md)
- Collection Postman : [`api/postman/`](api/postman/)
- Cahier des charges (source de vérité fonctionnelle) : `Documents/Arrondi - Cahier des charges v1.pdf`

## Avancement

| Jalon | État |
| --- | --- |
| J1 – Preuve de concept Enable Banking sur BoursoBank | ✅ |
| J2 – Moteur d'arrondis et profil BoursoBank | ✅ |
| J3 – Base de données, synchronisation et API HTTP | ✅ |
| J4 – PWA Vue | À faire |
| J5 – Déploiement sur le Mac mini | À faire |
| J6 – Mise en ligne pour la famille, ouverture des autres banques | À faire |

## Banques

| Banque | État |
| --- | --- |
| BoursoBank | Validée (J1) |
| Crédit Agricole, Crédit Mutuel, La Banque Postale | En attente d'une fiche de mapping validée sur un compte réel |

## Secrets

Ne jamais commiter : clés `.pem` Enable Banking, `.env`, `session.json`, clé maître (`master_key*`). Le [`.gitignore`](.gitignore) les exclut.
