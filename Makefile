.PHONY: build build-ui run test test-go test-ui test-package-verifier test-release-version \
	audit fmt check-format vet package package-host package-file verify-package \
	verify-package-host e2e clean

# Keep these values synchronized with manifest.yaml (server/manifest_test.go
# enforces it).
BIN := bin/kandev-plugin-azure-devops
VERSION := 0.1.0
STAGE := .build/stage
PKG_OUT := $(notdir $(BIN))-$(VERSION).tar.gz

# Sibling Kandev checkout pinned by .kandev-sdk-ref. plugin-pack runs inside
# Kandev's backend module so its tool-only dependencies stay out of this
# plugin's go.sum.
KANDEV_BACKEND := ../kandev/apps/backend

## Build the plugin binary for the host platform (development use). Kandev
## installs from `make package`/`package-host` output, not this.
build: build-ui
	mkdir -p bin
	go build -o $(BIN) ./server/...

build-ui:
	npm run build:ui

## Build + run. Kandev normally spawns this binary through the go-plugin
## handshake, so a manually started process has no host on the other end.
run: build
	./$(BIN)

test: test-go test-ui test-package-verifier test-release-version

test-go:
	go test ./...

test-ui:
	npm test

test-package-verifier:
	sh scripts/test-verify-package.sh

test-release-version:
	sh scripts/test-verify-release-version.sh

audit:
	npm audit --audit-level=high

fmt:
	gofmt -l .

check-format:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

vet:
	go vet ./...

## Cross-compile server/plugin-<goos>-<goarch>[.exe] for every platform in
## manifest.yaml's runtime.executables, stage manifest.yaml, assets/, and the
## built UI next to them, and pack the tree into $(PKG_OUT) with Kandev's
## cmd/plugin-pack.
package: build-ui
	rm -rf $(STAGE)
	mkdir -p $(STAGE)/server $(STAGE)/ui
	cp manifest.yaml $(STAGE)/manifest.yaml
	cp -r assets $(STAGE)/assets
	cp ui/bundle.js ui/plugin.css $(STAGE)/ui/
	GOOS=linux   GOARCH=amd64 go build -o $(STAGE)/server/plugin-linux-amd64       ./server
	GOOS=linux   GOARCH=arm64 go build -o $(STAGE)/server/plugin-linux-arm64       ./server
	GOOS=darwin  GOARCH=amd64 go build -o $(STAGE)/server/plugin-darwin-amd64      ./server
	GOOS=darwin  GOARCH=arm64 go build -o $(STAGE)/server/plugin-darwin-arm64      ./server
	GOOS=windows GOARCH=amd64 go build -o $(STAGE)/server/plugin-windows-amd64.exe ./server
	go -C $(KANDEV_BACKEND) run ./cmd/plugin-pack -dir $(abspath $(STAGE)) -out $(abspath $(PKG_OUT))
	rm -rf $(STAGE)
	@echo "Wrote $(PKG_OUT)"

## Package for the host platform only (plugin-pack -platform-only).
package-host: build-ui
	rm -rf $(STAGE)
	mkdir -p $(STAGE)/server $(STAGE)/ui
	cp manifest.yaml $(STAGE)/manifest.yaml
	cp -r assets $(STAGE)/assets
	cp ui/bundle.js ui/plugin.css $(STAGE)/ui/
	go build -o $(STAGE)/server/plugin-$$(go env GOOS)-$$(go env GOARCH)$$(go env GOEXE) ./server
	go -C $(KANDEV_BACKEND) run ./cmd/plugin-pack -dir $(abspath $(STAGE)) -out $(abspath $(PKG_OUT)) -platform-only
	rm -rf $(STAGE)
	@echo "Wrote $(PKG_OUT)"

## Verify the generated archive: checksums, every promised executable, and no
## development files.
verify-package: package
	@set -eu; \
		tmp="$$(mktemp -d)"; \
		trap 'rm -rf "$$tmp"' EXIT; \
		tar -xzf "$(PKG_OUT)" -C "$$tmp"; \
		sh scripts/verify-package.sh "$$tmp" full

verify-package-host: package-host
	@set -eu; \
		tmp="$$(mktemp -d)"; \
		trap 'rm -rf "$$tmp"' EXIT; \
		tar -xzf "$(PKG_OUT)" -C "$$tmp"; \
		sh scripts/verify-package.sh "$$tmp" host "$$(go env GOOS)-$$(go env GOARCH)"

## Print the archive name without building it (used by release automation).
package-file:
	@printf '%s\n' "$(PKG_OUT)"

# Packaged-host Playwright suite on desktop and phone projects. Starts a
# disposable Kandev host from ../kandev unless KANDEV_PLUGIN_E2E_URL is set.
e2e: verify-package-host
	KANDEV_PLUGIN_E2E_PACKAGE="$(abspath $(PKG_OUT))" npm run e2e

clean:
	rm -rf bin .build $(notdir $(BIN))-*.tar.gz
