COMPOSE = docker compose -f deploy/docker-compose.yml

.PHONY: deps-up deps-down test-go vet fmt

deps-up:
	$(COMPOSE) up -d --wait

deps-down:
	$(COMPOSE) down

test-go:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w internal
