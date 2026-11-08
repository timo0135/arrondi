# Commandes de développement (depuis le dossier api/).
GOOSE = go run github.com/pressly/goose/v3/cmd/goose@v3.26.0
DB   ?= postgres://arrondi:arrondi-dev@127.0.0.1:5432/arrondi?sslmode=disable

.PHONY: db-up db-down migrate migrate-down migrate-status sqlc test

db-up:            ## démarre PostgreSQL 18 dans Docker
	docker compose -f ../docker-compose.dev.yml up -d --wait

db-down:          ## arrête PostgreSQL (les données restent dans le volume)
	docker compose -f ../docker-compose.dev.yml down

migrate:          ## applique les migrations
	$(GOOSE) -dir db/migrations postgres "$(DB)" up

migrate-down:     ## annule la dernière migration
	$(GOOSE) -dir db/migrations postgres "$(DB)" down

migrate-status:
	$(GOOSE) -dir db/migrations postgres "$(DB)" status

sqlc:             ## régénère internal/store depuis db/queries (sqlc dans Docker, rien à installer)
	docker run --rm -v "$(CURDIR)":/src -w /src sqlc/sqlc:1.30.0 generate
	go mod tidy

test:
	go test ./...
