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

.PHONY: desktop desktop-app desktop-assets desktop-run build test-go test-web vet fmt test-ml-lifecycle

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
