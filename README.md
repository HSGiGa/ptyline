# ptyline

A lightweight terminal wrapper that reserves the last row of your terminal for a
configurable status bar — like tmux's status line, but **without** panes, tabs,
sessions, copy mode, or full terminal emulation. You keep your native shell and
native scrollback; ptyline just pins a bar at the bottom.

```text
Terminal Emulator → ptyline → PTY → fish / bash / zsh / vim / htop / …
```

> **Status: scaffolding.** The repository contains the full project skeleton
> (compilable Go stubs) plus the design docs and a sequenced implementation plan.
> No feature is implemented yet — start at [`docs/plans/`](docs/plans/).

## Quickstart

Requires **Go 1.26.1**. The required toolchain is declared in `go.mod`; use
`go version` to verify the active installation.

```sh
make bootstrap    # install pinned local gofumpt and golangci-lint
make build        # build dist/ptyline
make run ARGS='-- zsh' # run the (currently stubbed) wrapper around a command
make test         # run unit tests
make check        # format check, vet, tests, lint
make build-all    # cross-compile linux/darwin/windows
```

Run a single test:

```sh
make test-one PKG=./internal/reserved RUN=TestChildRows
```

## Intended CLI (see spec §14)

```sh
ptyline                 # run the configured shell or $SHELL
ptyline fish            # run fish inside the wrapper
ptyline -- bash         # everything after -- is the child command
ptyline init fish       # print the fish shell-integration script
ptyline --version
```

Config lives at `$XDG_CONFIG_HOME/ptyline/config.toml` (TOML; see
[`docs/config-reference.md`](docs/config-reference.md)).

## Documentation

- [`docs/`](docs/) — deep-dive design docs (state model, event bus, terminal
  safety, ANSI/OSC filter, layout, config, shell integration, platform, agents,
  testing).
- [`docs/plans/`](docs/plans/) — the implementation broken into small, ordered,
  independently-executable plans (`00` … `16`).
- [`ptyline-technical-spec.md`](ptyline-technical-spec.md) — the product/MVP spec.

## Platform support

One codebase, three binaries (`GOOS=linux|darwin|windows`). WSL2 is a **runtime
branch of the Linux binary**, not a separate target. Targets: Linux, WSL2,
macOS, Windows. See [`docs/platform-and-capabilities.md`](docs/platform-and-capabilities.md).
