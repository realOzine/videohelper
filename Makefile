COMPOSE = docker compose -f deploy/docker-compose.yml
GO_PACKAGES = ./cmd/... ./internal/... ./migrations/...

.PHONY: deps-up deps-down test-go test-web vet fmt build generate

deps-up:
	$(COMPOSE) up -d --wait

deps-down:
	$(COMPOSE) down

test-go:
	go test -race $(GO_PACKAGES)

test-web:
	node --test web/*.test.mjs

vet:
	go vet $(GO_PACKAGES)

fmt:
	gofmt -w cmd internal migrations

build:
	go build -o bin/videohelper ./cmd/videohelper

generate:
	@for package in workspace library index space pipeline usage; do sqlc generate -f internal/$$package/sqlc.yaml || exit 1; done
