# ProtoLab developer Makefile.
#
# CI uses the same targets (release.yml runs `make web`, `make release-local` and `make verify-dist`), so a
# local `make release-local` produces exactly what a merge to main publishes.
#
# Needs: GNU make, bash, git, go (see go.mod), node >= 20.19 + npm, tar, gzip, zip, unzip, curl.

SHELL         := /bin/bash
.DEFAULT_GOAL := help

BIN     := protolab
CMD     := ./cmd/protolab
BIN_DIR := bin
DIST    := dist
NOTICES := $(DIST)/THIRD_PARTY_NOTICES.md
WEB_DIR := frontend
WEB_OUT := internal/webui/dist

ifeq ($(OS),Windows_NT)
EXE := .exe
endif

# CI passes VERSION/COMMIT/DATE explicitly. Locally they come from git.
VERSION ?= $(or $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//'),dev)
COMMIT  ?= $(or $(shell git rev-parse HEAD 2>/dev/null),none)
DATE    ?= $(or $(shell TZ=UTC git show -s --date=format-local:%Y-%m-%dT%H:%M:%SZ --format=%cd HEAD 2>/dev/null),unknown)
VERSION := $(VERSION)
COMMIT  := $(COMMIT)
DATE    := $(DATE)

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

# Never ./... : after `npm ci` it would also match the Go sources inside frontend/node_modules.
GO_PKGS := ./cmd/... ./internal/...

TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

SHA256    := $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo "shasum -a 256")
TAR_OWNER := $(shell tar --version 2>/dev/null | grep -q 'GNU tar' && echo '--owner=0 --group=0 --numeric-owner')

# Frontend (React + Vite) -> internal/webui/dist, embedded with //go:embed all:dist.
WEB_SRC := $(shell find $(WEB_DIR)/src $(WEB_DIR)/public -type f 2>/dev/null) \
           $(wildcard $(WEB_DIR)/index.html $(WEB_DIR)/package.json $(WEB_DIR)/vite.config.ts \
                      $(WEB_DIR)/tsconfig*.json $(WEB_DIR)/.env.production)
WEB_STAMP  := $(WEB_OUT)/index.html
NODE_STAMP := $(WEB_DIR)/node_modules/.package-lock.json

.PHONY: help web web-dev build build-go run test test-go test-web vet fmt fmt-check lint check \
        verify-embed notices release-local verify-dist tidy clean

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-14s %s\n", $$1, $$2}'

$(NODE_STAMP): $(WEB_DIR)/package-lock.json
	cd $(WEB_DIR) && npm ci
	@touch $@

$(WEB_STAMP): $(WEB_SRC) $(NODE_STAMP)
	cd $(WEB_DIR) && npm run build
	@test -s $(WEB_STAMP) || { echo "ERROR: frontend build produced no $(WEB_STAMP)"; exit 1; }
	@touch $(WEB_STAMP)

web: $(WEB_STAMP) ## Build the web UI into internal/webui/dist (incremental; `make -B web` forces)

web-dev: $(NODE_STAMP) ## Vite dev server, /api is proxied to a running protolab on 127.0.0.1:7420
	cd $(WEB_DIR) && npm run dev

verify-embed: ## Fail unless the built web UI is embedded (guards against shipping the placeholder)
	@go list -f '{{join .EmbedFiles "\n"}}' ./internal/webui | grep -x 'dist/index.html' >/dev/null \
	  || { echo 'ERROR: dist/index.html is not embedded - run "make web" first'; exit 1; }

build: web ## Binary for this machine -> bin/protolab (TUI + embedded web UI)
	@$(MAKE) --no-print-directory verify-embed
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BIN)$(EXE) $(CMD)
	@echo "built $(BIN_DIR)/$(BIN)$(EXE) $(VERSION)"

build-go: ## Go only, embeds whatever is in internal/webui/dist
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BIN)$(EXE) $(CMD)

run: build ## Build and start the TUI; extra flags via ARGS="..."
	./$(BIN_DIR)/$(BIN)$(EXE) $(ARGS)

test: test-go test-web ## All tests

test-go: ## go test -race (the race detector needs cgo, the shipped binary does not)
	CGO_ENABLED=1 go test -race -count=1 $(GO_PKGS)

test-web: $(NODE_STAMP) ## vitest
	cd $(WEB_DIR) && npm test

vet: ## go vet
	go vet $(GO_PKGS)

fmt: ## gofmt -w
	gofmt -w cmd internal

fmt-check: ## Fail if gofmt would change anything
	@out="$$(gofmt -l cmd internal)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

lint: $(NODE_STAMP) ## go vet + tsc + eslint
	go vet $(GO_PKGS)
	cd $(WEB_DIR) && npm run typecheck && npm run lint -- --max-warnings=0

tidy: ## go mod tidy
	go mod tidy

check: fmt-check lint test ## gofmt, go vet, tsc, eslint and all tests (the core of the CI checks; CI adds smoke and cross builds)

notices: $(NODE_STAMP) ## Third-party licenses (Go modules of every target + web UI packages) -> dist/THIRD_PARTY_NOTICES.md
	scripts/notices.sh $(NOTICES) $(TARGETS)

release-local: web ## Cross-compile all targets into dist/ (tar.gz/zip + SHA256SUMS), exactly what CI publishes
	@$(MAKE) --no-print-directory verify-embed
	rm -rf $(DIST)
	mkdir -p $(DIST)/stage
	@$(MAKE) --no-print-directory notices
	@set -euo pipefail; \
	for t in $(TARGETS); do \
	  os="$${t%/*}"; arch="$${t#*/}"; \
	  name="$(BIN)_$(VERSION)_$${os}_$${arch}"; \
	  dir="$(DIST)/stage/$$name"; \
	  ext=""; if [ "$$os" = windows ]; then ext=".exe"; fi; \
	  echo "==> $$os/$$arch"; \
	  mkdir -p "$$dir"; \
	  CGO_ENABLED=0 GOOS="$$os" GOARCH="$$arch" go build -trimpath -ldflags '$(LDFLAGS)' -o "$$dir/$(BIN)$$ext" $(CMD); \
	  cp README.md "$(NOTICES)" "$$dir/"; \
	  for f in LICENSE*; do if [ -e "$$f" ]; then cp "$$f" "$$dir/"; fi; done; \
	  if [ "$$os" = windows ]; then \
	    (cd $(DIST)/stage && zip -q -X -r "../$$name.zip" "$$name"); \
	  else \
	    COPYFILE_DISABLE=1 tar $(TAR_OWNER) -C $(DIST)/stage -cf - "$$name" | gzip -n -9 > "$(DIST)/$$name.tar.gz"; \
	  fi; \
	done
	cd $(DIST) && $(SHA256) *.tar.gz *.zip > SHA256SUMS
	@echo; cat $(DIST)/SHA256SUMS

verify-dist: ## Check the output of release-local (archives, checksums, notices, build flags, a running binary)
	scripts/verify-dist.sh $(DIST) $(VERSION) $(TARGETS)

clean: ## Remove bin/, dist/ and the built web UI (keeps the tracked placeholder)
	rm -rf $(BIN_DIR) $(DIST)
	find $(WEB_OUT) -mindepth 1 ! -name README.md -delete
