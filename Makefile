COMPOSE = docker compose -f deploy/docker-compose.yml
GO_PACKAGES = ./internal/... ./migrations/...

.PHONY: deps-up deps-down test-go vet fmt

deps-up:
	$(COMPOSE) up -d --wait

deps-down:
	$(COMPOSE) down

test-go:
	go test -race $(GO_PACKAGES)

vet:
	go vet $(GO_PACKAGES)

fmt:
	gofmt -w internal
