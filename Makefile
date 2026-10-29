GO_PACKAGES = . ./desktop/... ./cmd/... ./internal/...
DESKTOP_OS = $(shell go env GOOS)
DESKTOP_ARCH = $(shell go env GOARCH)
ifeq ($(DESKTOP_OS),darwin)
DESKTOP_BINARY = .local/desktop-build/darwin-$(DESKTOP_ARCH)/VideoHelper.app/Contents/MacOS/VideoHelper
DESKTOP_ENV = MACOSX_DEPLOYMENT_TARGET=13.0 CGO_CFLAGS='-O2 -g -mmacosx-version-min=13.0' CGO_LDFLAGS='-O2 -g -mmacosx-version-min=13.0' MYGO_LDFLAGS='-macos=13.0 -linkmode=external -extldflags "-Wl,-sectcreate,__TEXT,__info_plist,$(CURDIR)/desktop/Info.plist"'
else ifeq ($(DESKTOP_OS),windows)
DESKTOP_BINARY = .local/desktop-build/windows-$(DESKTOP_ARCH)/VideoHelper.exe
else
DESKTOP_BINARY = .local/desktop-build/linux-$(DESKTOP_ARCH)/videohelper
endif

# Validate all release goals while parsing, before even unrelated recipes can run.
# SQLite uses CGO: release builds require the target's native OS and architecture.
ifneq ($(filter release,$(MAKECMDGOALS)),)
RELEASE_PLATFORMS := $(filter-out release,$(MAKECMDGOALS))
ifneq ($(filter-out mac windows linux,$(RELEASE_PLATFORMS)),)
$(error Unknown release platform; use make release [mac|windows|linux])
endif
ifneq ($(word 2,$(RELEASE_PLATFORMS)),)
$(error Select exactly one release platform: mac windows or linux)
endif
RELEASE_HOST_OS := $(shell go env GOHOSTOS)
RELEASE_HOST_ARCH := $(shell go env GOHOSTARCH)
RELEASE_OS := $(if $(RELEASE_PLATFORMS),$(if $(filter mac,$(RELEASE_PLATFORMS)),darwin,$(RELEASE_PLATFORMS)),$(RELEASE_HOST_OS))
ifneq ($(filter $(RELEASE_OS),darwin windows linux),$(RELEASE_OS))
$(error Unsupported release OS: $(RELEASE_OS))
endif
ifneq ($(RELEASE_OS),$(RELEASE_HOST_OS))
$(error Release requires native $(RELEASE_OS); current host is $(RELEASE_HOST_OS). Cross-compiling CGO releases is unsupported)
endif
ifneq ($(DESKTOP_OS)/$(DESKTOP_ARCH),$(RELEASE_HOST_OS)/$(RELEASE_HOST_ARCH))
$(error Unset cross-compiling GOOS/GOARCH: release requires native $(RELEASE_HOST_OS)/$(RELEASE_HOST_ARCH))
endif
RELEASE_ARCH := $(RELEASE_HOST_ARCH)
ifeq ($(RELEASE_OS),darwin)
ifneq ($(RELEASE_ARCH),arm64)
$(error mac release currently supports only arm64; current host is $(RELEASE_ARCH))
endif
else
ifneq ($(RELEASE_ARCH),amd64)
$(error $(RELEASE_OS) release currently supports only amd64; current host is $(RELEASE_ARCH))
endif
endif
endif

.PHONY: desktop desktop-app desktop-assets desktop-run build test-go test-web vet fmt test-ml-lifecycle release mac windows linux

desktop-assets:
	npm --prefix web run build

desktop: desktop-app
ifeq ($(DESKTOP_OS),darwin)
	cmp desktop/Info.plist .local/desktop-build/darwin-$(DESKTOP_ARCH)/VideoHelper.app/Contents/Info.plist
	cmp desktop/AppIcon.icns .local/desktop-build/darwin-$(DESKTOP_ARCH)/VideoHelper.app/Contents/Resources/AppIcon.icns
	cmp desktop/CodeResources .local/desktop-build/darwin-$(DESKTOP_ARCH)/VideoHelper.app/Contents/_CodeSignature/CodeResources
endif
	mkdir -p bin
	cp $(DESKTOP_BINARY) bin/VideoHelper$(if $(filter windows,$(DESKTOP_OS)),.exe,)

desktop-app:
	$(DESKTOP_ENV) CGO_ENABLED=1 go run github.com/egoist/mygo/cmd/mygo@v0.7.1 build -skip-dmg -skip-notarize

desktop-run:
	$(DESKTOP_ENV) CGO_ENABLED=1 go run github.com/egoist/mygo/cmd/mygo@v0.7.1 dev

release:
	$(DESKTOP_ENV) CGO_ENABLED=1 go run github.com/egoist/mygo/cmd/mygo@v0.7.1 build -platform $(RELEASE_OS)/$(RELEASE_ARCH) -o Release -skip-dmg -skip-notarize

# Platform words are selectors for release, never separate build jobs.
mac windows linux:
	$(if $(filter release,$(MAKECMDGOALS)),:,$(error Use make release $@))

build: desktop

test-go:
	go test -race $(GO_PACKAGES)

test-web:
	npm --prefix web test

vet:
	go vet $(GO_PACKAGES)

fmt:
	gofmt -w assets.go assets_test.go desktop cmd internal

test-ml-lifecycle:
	python3 ml/tools/check_lifecycle.py
