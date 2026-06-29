# ptyline — build & dev tasks
# Requires the Go toolchain (go 1.26+). See README.md for installation.

BINARY      := ptyline
CONFIG      ?= $(CURDIR)/config/config.toml
PKG         := ./cmd/ptyline
DIST        := dist
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)
GO_BUILD_FLAGS := -buildvcs=false

GO          ?= go
TOOLS_BIN   := $(CURDIR)/.tools/bin
GOCACHE     ?= $(CURDIR)/.cache/go-build
GOLANGCI_LINT_CACHE ?= $(CURDIR)/.cache/golangci-lint
GOLANGCI_LINT_VERSION := v2.12.2
GOFUMPT_VERSION := v0.9.2
GOLANGCI_LINT := $(TOOLS_BIN)/golangci-lint
GOFUMPT := $(TOOLS_BIN)/gofumpt

export GOCACHE GOLANGCI_LINT_CACHE

.DEFAULT_GOAL := build

.PHONY: bootstrap
bootstrap: tools ## Install pinned local development tools and download module dependencies
	$(GO) mod download

.PHONY: tools
tools: ## Install pinned local lint and formatting tools into .tools/bin
	@mkdir -p $(TOOLS_BIN)
	@test -x $(GOLANGCI_LINT) || GOBIN=$(TOOLS_BIN) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@test -x $(GOFUMPT) || GOBIN=$(TOOLS_BIN) $(GO) install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)

.PHONY: build
build: ## Build the binary for the host platform into dist/
	@mkdir -p $(DIST)
	$(GO) build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY) $(PKG)

.PHONY: build-all
build-all: ## Cross-compile linux, darwin, windows binaries
	@mkdir -p $(DIST)
	GOOS=linux   GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-amd64   $(PKG)
	GOOS=darwin  GOARCH=arm64 $(GO) build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-darwin-arm64  $(PKG)
	GOOS=windows GOARCH=amd64 $(GO) build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-windows-amd64.exe $(PKG)

.PHONY: run
run: build ## Build and run with fish integration (use ARGS="..." to override)
	@if [ -n "$(ARGS)" ]; then \
		PATH="$(CURDIR)/$(DIST):$$PATH" $(DIST)/$(BINARY) --config "$(CONFIG)" $(ARGS); \
	else \
		PATH="$(CURDIR)/$(DIST):$$PATH" $(DIST)/$(BINARY) --config "$(CONFIG)" fish -C 'ptyline init fish | source'; \
	fi

.PHONY: test
test: ## Run the full test suite
	$(GO) test ./...

.PHONY: test-one
test-one: ## Run a single test: make test-one PKG=./internal/proxy RUN=TestAnsiFilter
	$(GO) test -v -run '$(RUN)' $(PKG)

.PHONY: cover
cover: ## Run tests with coverage and write coverage.txt
	$(GO) test -coverprofile=coverage.txt -covermode=atomic ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: tools ## Run the pinned local golangci-lint
	$(GOLANGCI_LINT) run

.PHONY: fmt
fmt: tools ## Format the codebase with the pinned gofumpt
	$(GOFUMPT) -extra -w .

.PHONY: fmt-check
fmt-check: tools ## Verify formatting without modifying files
	@test -z "$$($(GOFUMPT) -extra -l .)" || (echo "Run 'make fmt' to format these files:"; $(GOFUMPT) -extra -l .; exit 1)

.PHONY: check
check: fmt-check vet test lint ## Run the local development validation suite

.PHONY: tidy
tidy: ## Sync go.mod / go.sum
	$(GO) mod tidy

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(DIST) coverage.txt coverage.html

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
