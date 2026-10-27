COMPOSE = docker compose -f deploy/docker-compose.yml
GO_PACKAGES = . ./desktop/... ./cmd/... ./internal/... ./migrations/...
ML_PROFILE ?= wemm-mps
ML_PROFILE_DIR = ml/profiles/$(ML_PROFILE)
ML_PORT ?= 8000
DESKTOP_OS = $(shell go env GOOS)
DESKTOP_ARCH = $(shell go env GOARCH)
ifeq ($(DESKTOP_OS),darwin)
DESKTOP_BINARY = .local/desktop-build/darwin-$(DESKTOP_ARCH)/VideoHelper.app/Contents/MacOS/VideoHelper
else ifeq ($(DESKTOP_OS),windows)
DESKTOP_BINARY = .local/desktop-build/windows-$(DESKTOP_ARCH)/VideoHelper.exe
else
DESKTOP_BINARY = .local/desktop-build/linux-$(DESKTOP_ARCH)/videohelper
endif

.PHONY: deps-up deps-down test-go test-web vet fmt build generate
.PHONY: ml-install ml-prepare ml-serve ml-status test-ml-lifecycle
.PHONY: desktop desktop-app desktop-assets desktop-run

desktop-assets:
	npm --prefix web run build

desktop: desktop-app
	mkdir -p bin
	cp $(DESKTOP_BINARY) bin/VideoHelper$(if $(filter windows,$(DESKTOP_OS)),.exe,)

desktop-app:
	CGO_ENABLED=1 go run github.com/egoist/mygo/cmd/mygo@v0.7.1 build -skip-dmg -skip-notarize

desktop-run:
	CGO_ENABLED=1 go run github.com/egoist/mygo/cmd/mygo@v0.7.1 dev

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
