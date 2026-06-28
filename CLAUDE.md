# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Current State

This repository is a **scaffold**: a compilable Go skeleton (stdlib-only stubs), the full docs tree,
and a sequenced implementation plan. No feature is implemented yet — every `internal/**` file is a stub
with real type/interface signatures and `TODO scaffold (plan NN)` bodies. Start implementing from
[`docs/plans/`](docs/plans/) (`00` … `16`, in dependency order).

**The implementation language is Go** (spec §22), module `github.com/hsgiga/ptyline`, targeting Go
1.23+. The Go toolchain lives at `/usr/local/go/bin` (not on `$PATH` by default — prepend it).

Sources of truth, in order:
- [`ARCHITECTURE.md`](ARCHITECTURE.md) — the one-page architecture map and the binding invariants.
- [`docs/`](docs/) — deep-dive design docs; **read [`docs/terminal-safety.md`](docs/terminal-safety.md)
  before touching `internal/terminal`, `internal/pty`, or `internal/proxy`.**
- [`docs/plans/`](docs/plans/) — the implementation broken into small, ordered, executable slices.
- [`ptyline-technical-spec.md`](ptyline-technical-spec.md) — product/MVP spec.
- [`ARCHITECTURE.md`](ARCHITECTURE.md) — future-ready architecture notes.

### Commands

```sh
export PATH=$PATH:/usr/local/go/bin      # toolchain is here, not on PATH
make build        # → dist/ptyline
make test         # go test ./...
make test-one PKG=./internal/reserved RUN=TestChildRows
make vet          # go vet ./...
make lint         # golangci-lint (must be installed separately)
make fmt          # gofumpt/gofmt
make build-all    # cross-compile linux/darwin/windows
```

### Code layout

`cmd/ptyline` (entrypoint) → `internal/app` (wiring + CLI). Core packages: `event` (typed bus),
`proxy` (event loop + ANSI/OSC filter), `terminal` (real terminal), `pty` (child PTY), `status`
(+`layout`/`renderer`/`width`/`theme`/`style`/`icons`), `modules`, `config`, `runtimeenv`, `platform`,
`shellintegration`, `reserved`, `diagnostics`. Expected external deps (added per-plan, not yet present):
`creack/pty`, `golang.org/x/term`, `mattn/go-runewidth`, `BurntSushi/toml`, `muesli/termenv`.

When implementing a plan, replace its `TODO scaffold (plan NN)` markers (grep-able) and add the
dependency it calls for, then run `make tidy`.

## What the project is

A lightweight PTY wrapper that reserves the **last visible terminal row** for a persistent,
configurable status bar — tmux's status line without panes, tabs, sessions, copy mode, or full VT
emulation (see Non-Goals, §3). Native terminal-emulator scrollback is preserved; the app does **not**
implement its own scrollback.

Pipeline: `Terminal Emulator → ptyline → PTY → shell/child program`.

## Core architectural invariants

These are the load-bearing rules that make the whole design work. Violating any of them corrupts the
user's terminal. They span multiple components, so internalize them before editing:

1. **Child PTY height = `rows - reserved` in the normal screen** (`max(rows-1,1)` for MVP), and **full
   `rows` while the alternate screen is active** (bar hidden — §11). Computed only via
   `reserved.Area.ChildRows`. Never hardcode `rows-1`.
2. **Normal screen → scroll region `1..rows-1`** (`ESC [ 1 ; {rows-1} r`); **alternate screen → region
   reset** (child owns every row).
3. **The ANSI filter protects the last row in the normal screen only.** It rewrites a bare `ESC [ r` to
   `ESC [ 1 ; {rows-1} r` and clamps any region overlapping the reserved row; in the alt screen it does
   **not** clamp. It tracks alt-screen enter/leave and intercepts **whitelisted** OSC 777 messages
   (`cwd`/`exit_code`/`duration_ms`/`command`, ≤8KiB, no control chars; CSI buffer ≤4KiB) without
   forwarding them. Intentionally **not** a full terminal emulator (§8.4).
4. **The bar renderer uses absolute cursor positioning and never prints a trailing newline.** Flow:
   save cursor → move to last row → clear line → write bar → reset style → restore cursor. A stray
   newline pushes the bar into scrollback (§8.6, §10.3).
5. **Terminal state must always be restored** — on normal exit, `SIGINT`/`SIGTERM`, child exit, or
   init failure after the terminal was already modified. Cleanup = reset scroll region, reset
   attributes, restore cursor, restore terminal mode, show cursor (§8.1, §15).
6. **The wrapper exits with the child's exit code** (§8.2).

## Component model

Driven by a single event loop (§8.3) that multiplexes: stdin, PTY output, resize events, status-timer
ticks, child exit, and signals. Key components and their boundaries:

- **Terminal Controller** (§8.1) — owns the *real* terminal: raw mode, size, scroll region, cursor,
  cleanup.
- **PTY Supervisor** (§8.2) — creates the child PTY, spawns the shell/command, owns its
  session/controlling-tty/process-group + signal handling, applies the reserved-rows resize rule,
  returns the exit code.
- **IO Proxy / event loop + serialized writer** (§8.3) — forwards stdin→PTY and PTY→stdout (through the
  ANSI filter); a single serialized writer emits both filtered child output and bar frames and
  schedules redraws at safe boundaries.
- **Status State** (§8.5) — structured state read by the renderer; updated by modules, resize, child
  lifecycle, and OSC shell-integration messages.
- **Bar Renderer + Layout/Theme/Style** (§8.6–§8.9) — blocks anchored left/center/right, widths in
  **terminal cells** (`auto | fill | N | N%`). The renderer measures **display width**, not byte
  length and not rune count (Unicode-width awareness is mandatory).
- **Module System** (§8.7) — each module has a refresh interval, timeout, cached value, fallback, and
  render fn. **Expensive work (e.g. `git status`) runs on its own interval with a timeout; every
  redraw reads the cached value.** Never shell out per redraw.

## Cross-cutting design decisions

- **Platform model (§4):** **MVP targets Linux + WSL/WSL2 only**; macOS and Windows/ConPTY are
  post-MVP (§19) and must not delay the MVP (`darwin`/`windows` files are build-tagged stubs). One
  codebase, fan-out via `GOOS`. **WSL2 is a runtime branch inside the Linux binary, not a separate
  target.** Detect the environment once into a normalized profile → capability flags (`unix_pty`,
  `vt_sequences`, `linux_procfs`, …) → backend selection. **Components depend on capabilities, never on
  raw OS-name checks.**
- **Alternate screen (§11):** MVP policy is **hide the bar** — on entry, reset the scroll region and
  resize the child to full `rows`; on exit, resize to `rows-1`, restore the `1..rows-1` region, redraw.
  Not configurable in the MVP. The ANSI filter does **not** clamp margins while the alt screen is active.
- **Resize (§12):** debounce (~30–50ms) → re-query size → mode-specific: alt screen resizes child to
  full `rows` (region reset); normal screen resizes to `rows-1`, sets `1..rows-1`, redraws.
- **Serialized writer (§8.3):** all real-terminal writes (filtered child output + bar frames) go
  through one writer; a bar frame is never inserted mid child-write; redraws are ≤20Hz and skipped if
  unchanged.
- **PTY supervisor (§8.2):** owns the child session/controlling-tty/process-group; forwards
  Ctrl-C/Ctrl-Z as PTY bytes; reaps the process group on controlled shutdown.
- **Shell integration (§9):** optional — ptyline works with any shell/command without it. The Go side
  is shell-agnostic (one OSC 777 parser + `ShellState` updater); each shell is just an embedded script
  template in `shellintegration/templates/{bash,zsh,fish}.sh` that emits whitelisted OSC 777 metadata.
  `ptyline init <shell>` prints the template.
- **Config (§13, §13.1):** TOML at `$XDG_CONFIG_HOME/ptyline/config.toml` (fallback `~/.config/...`),
  `config_version` required. Bar layout uses `bar.format` (placeholder template, e.g.
  `"{cwd} {hostname} || {time}"`) or `[[bar.row]]` for multi-line; invalid config is a startup error
  naming the key. Deliberately **not** Markdown.
- **Security (§17):** custom-command modules execute local shell commands and the config is trusted
  user input — but OSC messages must be parsed strictly and must never trigger arbitrary execution.

## MVP scope vs. later

Build the MVP first (§18): run shell in PTY, raw mode, bidirectional proxy, reserved-rows sizing,
scroll region, one-line bar with left/center/right blocks (fixed + auto width), basic ANSI fg/bg, one
default theme, ASCII-safe fallback, time/hostname/static-text modules, minimal ANSI filter, alt-screen
detection (hide bar, give child full height; restore on exit), the serialized terminal writer, Unix
session/process-group/signal handling, optional shell integration, **Linux/WSL only**. Defer git,
command duration, battery, themes/presets, Powerline/Nerd-Font styling, mouse-aware blocks, and native
macOS/Windows backends to post-MVP (§19).

Acceptance criteria are enumerated in §20 (plus the §20.1 verification matrix) — treat them as the
definition of done for the MVP.
