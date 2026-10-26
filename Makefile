COMPOSE = docker compose -f deploy/docker-compose.yml
GO_PACKAGES = ./cmd/... ./internal/... ./migrations/...
ML_PROFILE ?= wemm-mps
ML_PROFILE_DIR = ml/profiles/$(ML_PROFILE)
ML_PORT ?= 8000

.PHONY: deps-up deps-down test-go test-web vet fmt build generate
.PHONY: ml-install ml-prepare ml-serve ml-status test-ml-lifecycle

deps-up:
	$(COMPOSE) up -d --wait

deps-down:
	$(COMPOSE) down

test-go:
	go test -race $(GO_PACKAGES)

test-web:
	npm --prefix web test

vet:
	go vet $(GO_PACKAGES)

fmt:
	gofmt -w cmd internal migrations

build:
	go build -o bin/videohelper ./cmd/videohelper

generate:
	@for package in workspace library index space pipeline usage; do sqlc generate -f internal/$$package/sqlc.yaml || exit 1; done

ml-install:
	uv sync --project $(ML_PROFILE_DIR) --locked --no-dev --no-editable

ml-prepare:
	$(ML_PROFILE_DIR)/.venv/bin/videohelper-ml prepare --models $(ML_PROFILE_DIR)/models.yaml

ml-serve:
	$(ML_PROFILE_DIR)/.venv/bin/videohelper-ml serve --models $(ML_PROFILE_DIR)/models.yaml --port $(ML_PORT)

ml-status:
	curl --fail --silent --show-error http://127.0.0.1:$(ML_PORT)/readyz

test-ml-lifecycle:
	python3 ml/tools/check_lifecycle.py
