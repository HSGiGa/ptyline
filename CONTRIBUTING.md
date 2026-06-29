# Contributing to ptyline

Thanks for your interest in contributing! ptyline is a lightweight PTY wrapper
that reserves the last terminal row for a configurable status bar. This guide
covers the local workflow and the few rules that keep the project safe to hack on.

## Prerequisites

- **Go 1.26+** (the minimum is declared in `go.mod`; check with `go version`).
- A Unix-like environment. The MVP targets **Linux and WSL2**; macOS and Windows
  are post-MVP build-tagged stubs (they compile but do not yet run).

## Getting started

```sh
make bootstrap     # download modules + install pinned gofumpt & golangci-lint into .tools/
make build         # build dist/ptyline
make run ARGS='-- bash'   # run the wrapper around a command
```

## Before you open a pull request

Run the full local validation suite — this mirrors CI:

```sh
make check         # fmt-check + vet + test + lint
```

Or individually:

```sh
make fmt           # format with the pinned gofumpt
make vet           # go vet ./...
make test          # go test ./...
make lint          # pinned golangci-lint
go test -race ./...   # the event loop is concurrent — always run with -race
```

CI runs `go vet`, `go test -race`, `make fmt-check`, and `make lint` across
`GOOS=linux|darwin|windows`. Keep all of them green.

## Read this before touching terminal internals

ptyline writes directly to your live terminal, so a bug here can corrupt a user's
session. **Before editing `internal/terminal`, `internal/pty`, or
`internal/proxy`, read [`docs/terminal-safety.md`](docs/terminal-safety.md)** —
it lists the non-negotiable invariants (reserved-rows sizing, scroll region,
alt-screen handling, the single serialized writer, and always restoring terminal
state). [`ARCHITECTURE.md`](ARCHITECTURE.md) is the one-page map of how the
components fit together.

A few load-bearing rules in short:

- Child PTY height comes only from `reserved.Area.ChildRows` — never hardcode
  `rows-1`.
- All real-terminal writes go through the single `proxy.TerminalWriter`.
- The bar is drawn with absolute positioning and **no trailing newline**.
- Terminal state is always restored on exit, signal, or init failure.
- OSC input is parsed strictly and never executed; see
  [`SECURITY.md`](SECURITY.md).

## Code style

- Format with `gofumpt` (`make fmt`); `make fmt-check` must pass.
- Match the surrounding code's naming, comment density, and idiom.
- Keep packages decoupled: the wiring lives in `internal/app`; the other packages
  do not import it. See the layout note in
  [`CLAUDE.md`](CLAUDE.md#code-layout).
- Add tests for new behavior. The renderer, layout, filter, and config packages
  have good examples to follow.

## Commit & PR conventions

- Write focused commits with clear messages explaining the *why*.
- Reference any related issue.
- Describe how you verified the change (which `make` targets, manual terminal
  testing if it touches rendering).

## Reporting bugs & security issues

- Functional bugs: open a GitHub issue with your platform, `ptyline --version`,
  config snippet, and reproduction steps.
- Security vulnerabilities: **do not** open a public issue — follow
  [`SECURITY.md`](SECURITY.md).
