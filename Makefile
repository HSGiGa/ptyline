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
HOST_OS     := $(shell $(GO) env GOOS)
HOST_ARCH   := $(shell $(GO) env GOARCH)

# Builds are produced natively on each target platform — there is no
# cross-compilation. The macOS system-metric modules (cpu/memory/load/battery)
# call mach/IOKit through cgo, so darwin must build with CGO enabled; the
# linux/windows paths are pure Go and build statically.
ifeq ($(HOST_OS),darwin)
CGO_ENABLED ?= 1
else
CGO_ENABLED ?= 0
endif
export CGO_ENABLED

# Install destination. Default to a user-writable directory so `make install`
# never needs sudo:
#   - macOS + Homebrew: the brew prefix bin (/opt/homebrew/bin on Apple Silicon,
#     /usr/local/bin on Intel) — user-owned and already on $PATH.
#   - otherwise ~/.local/bin (XDG convention; add it to $PATH if missing).
# Override with `make install BINDIR=/usr/local/bin` (needs sudo) or set DESTDIR
# for packaging.
BREW_BIN := $(shell brew --prefix 2>/dev/null)/bin
ifneq ($(wildcard $(BREW_BIN)/.),)
BINDIR ?= $(BREW_BIN)
else
BINDIR ?= $(HOME)/.local/bin
endif

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
build: ## Build the binary for the host platform into dist/ (native; cgo on darwin)
	@mkdir -p $(DIST)
	$(GO) build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY) $(PKG)

.PHONY: dist
dist: ## Build a release binary for the host platform, named ptyline-<os>-<arch> (no cross-compile)
	@mkdir -p $(DIST)
	$(GO) build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-$(HOST_OS)-$(HOST_ARCH) $(PKG)

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

# Lint every target platform, not just the host. Build tags mean each GOOS
# compiles a different set of *_linux.go / *_darwin.go / *_other.go files, so a
# host-only lint (e.g. darwin) never sees issues in files that only build
# elsewhere (e.g. an unused var on linux). The linux path is pure Go and
# cross-lints from any host with CGO disabled; the darwin path uses cgo (IOKit)
# and can only be linted natively on a darwin host. CI runs this same target on a
# macOS runner so local and CI check the identical set of files. Windows (ConPTY)
# is a deferred future feature, so its *_windows.go stubs are not linted here.
LINT_PLATFORMS := linux/amd64
ifeq ($(HOST_OS),darwin)
LINT_PLATFORMS += darwin/arm64
endif

.PHONY: lint
lint: tools ## Run the pinned golangci-lint across all target platforms (darwin requires a darwin host)
	@for p in $(LINT_PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		if [ "$$os" = darwin ]; then cgo=1; else cgo=0; fi; \
		echo "==> lint $$os/$$arch (CGO_ENABLED=$$cgo)"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=$$cgo $(GOLANGCI_LINT) run || exit 1; \
	done

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

.PHONY: install
install: build ## Install the binary into $(DESTDIR)$(BINDIR) (user-writable, no sudo)
	install -d $(DESTDIR)$(BINDIR)
	install -m 0755 $(DIST)/$(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	@echo "installed $(BINARY) -> $(DESTDIR)$(BINDIR)/$(BINARY)"

.PHONY: uninstall
uninstall: ## Remove the installed binary from $(DESTDIR)$(BINDIR)
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY)

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(DIST) coverage.txt coverage.html

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
