# Architecture

The canonical map of ptyline: package boundaries, data flow, and the rules that
keep the user's terminal intact.

## The core idea

ptyline sits between the terminal emulator and the user's shell. In the normal
screen it runs the shell in a pseudo-terminal that is **one or more rows
shorter** than the real terminal, sets the real terminal's scroll region to
exclude those rows, and draws its own status bar on them. In the alternate
screen, the bar is hidden and the child gets the full terminal height.

```text
┌──────────────────────────────┐
│ child PTY output (rows 1..n-1)│  ← normal screen: child height = rows - reserved
│ prompt                        │
├──────────────────────────────┤
│ cwd | git | time   (row n)    │  ← ptyline's status bar (reserved area)
└──────────────────────────────┘
```

## Data flow (one direction)

The central design rule (ARCHITECTURE.md §2): the renderer never queries git/shell/modules.
Everything flows one way into a prepared `StatusState`, which the renderer reads.

```text
Input sources → Events → Normalized state → Layout → Renderer → Terminal output

stdin ─┐
PTY  ──┤                                  ┌─ StatusState ─┐
resize ┼─► event.Bus ─► proxy.Loop ─────► │  terminal     │ ─► layout/renderer ─► bar rows
timer ─┤    (single)    (only mutator)    │  shell        │     (+ click zones)
OSC  ──┤                                  │  modules      │
signal ┤                                  │  diagnostics  │
reload ┘                                  └───────────────┘
```

## Components

| Package | Owns | Notes |
|---|---|---|
| `cmd/ptyline` | entrypoint | thin; calls `app.Run` |
| `internal/app` | wiring + CLI | the only place that knows how pieces fit |
| `internal/app/bar` | config-to-render specs | turns config rows, templates, themes, icons, and animations into renderer inputs |
| `internal/command` | command-display lifecycle | debounces active/done command visibility and animation state |
| `internal/event` | `AppEvent` + `Bus` | typed, sealed event set (spec §24.1) |
| `internal/proxy` | event loop, ANSI/OSC filter, serialized writer | **only** state mutator; protects the reserved row; one writer for child output + bar frames |
| `internal/terminal` | the **real** terminal | raw mode, size, scroll region, guaranteed restore |
| `internal/pty` | the **child** PTY | spawn, resize, exit code; owns the child session/process-group + signals; Unix backend; Windows/ConPTY deferred |
| `internal/status` | `StatusState`, `Module` | normalized state + typed module values |
| `internal/status/{layout,renderer,width,theme,style,icons,formatting}` | the bar UI | layout engine, display width, theme/style/icon resolution, separator cleanup |
| `internal/modules` | built-in modules | time/date, hostname/user/runtime/shell/env/cwd, ssh, command, git, exec, template, system metrics |
| `internal/config` | TOML schema/loader/migrate | versioned config, overlays, nearest project `.ptyline` discovery |
| `internal/runtimeenv` | runtime profile + capabilities | detect once; components query capabilities |
| `internal/platform` | OS-specific detection | build-tagged; WSL = Linux runtime branch |
| `internal/shellintegration` | OSC contract + init scripts | cwd/env/command/exit/duration/SSH via OSC 777; **shell-agnostic — adding a shell is a template file, never Go logic** |
| `internal/reserved` | reserved-area math | single source of truth for "rows − reserved" |
| `internal/diagnostics` | health/debug state | lightweight record type; replay/doctor tooling is deferred |

## Load-bearing invariants (read before touching terminal/pty/proxy)

These corrupt the user's terminal if broken.

1. **Child PTY height = `terminalRows − reserved.Rows`** in the normal screen
   (never < 1), and **full `terminalRows`** while the alternate screen is active.
   Computed only via `reserved.Area.ChildRows` (spec §6, §11).
2. **Normal screen → scroll region `1..childRows`** (`CSI 1 ; childRows r`).
   **Alternate screen → scroll region reset and bar hidden** (the child owns every
   row). Re-applied on every transition and resize (spec §6, §10, §11, §12).
3. **The ANSI/OSC filter protects the reserved row(s) in the normal screen only:**
   rewrites bare `CSI r` and clamps regions overlapping the reserved rows; in the
   alternate screen it does **not** clamp. It tracks alt-screen and **consumes**
   whitelisted OSC 777 messages (spec §8.4, §9).
4. **One serialized terminal writer** carries both filtered child output and
   complete bar frames — a bar frame is never inserted mid child-write, and child
   bytes are never dropped/duplicated/reordered (spec §8.3).
5. **The bar is drawn with absolute positioning and never a trailing newline:**
   save cursor → move to bar row → clear line → write → reset → restore (spec §8.6).
6. **Terminal state is always restored** — normal exit, signals, child exit, or
   init failure after modifying state; the child **process group** is reaped on
   controlled shutdown (spec §8.1, §8.2, §15).
7. **ptyline exits with the child's exit code** (spec §8.2).

## Configuration & Module Model

Config is TOML, versioned by `config_version`, and loaded over built-in defaults.
The sample config carries `#:schema ./config.schema.json`; the Go source of truth
is `internal/config/schema.go`.

The effective config is layered:

```text
built-in defaults → base config → optional CLI --ptyline overlay → nearest project .ptyline
```

Project overlays are visual/profile overlays. They may change bar rows, module
presentation, themes, icons, and styles, but they must not choose the child
command or introduce command execution. Reloads (`ptyline --reload` or project
overlay changes after `cwd` OSC updates) rebuild rows, visuals, and module
lifecycles without restarting the child shell.

Modules publish cached `status.ModuleSnapshot` values. Slow or external work
never runs during rendering:

- interval modules: time/date, git, exec, and system metrics;
- event-driven modules: cwd/env/command/SSH from shell-integration OSC;
- static/computed modules: hostname, user, runtime, shell labels, templates.

Probe-driven system modules (`cpu`, `memory`, `disk`, `load`, `battery`) hide
when unavailable and reconcile on reload. macOS providers use mach/IOKit through
cgo; Linux providers use procfs/sysfs/statfs-style sources.

## Platform Scope & Build Matrix

**Current readiness target: Linux, Linux/WSL, and macOS.** WSL2 is a runtime
branch of the Linux binary, not a separate target. macOS uses the shared Unix PTY
backend and has native system-metric providers (cpu/memory/load/battery) backed
by mach/IOKit. Windows/ConPTY is deferred future work; the `windows` files remain
build-tagged stubs.

Binaries are built **natively on each target platform — there is no
cross-compilation.** The macOS metric providers call mach/IOKit through cgo, so
the darwin build requires `CGO_ENABLED=1` (a C toolchain), which makes
cross-compiling the darwin binary from another host impractical; building per
platform sidesteps that. `make build` (host binary) and `make dist`
(`ptyline-<os>-<arch>`) drive the host build; the Makefile enables cgo on darwin
and keeps linux/windows pure-Go static builds.

```text
GOOS=linux   → Linux binary  (Unix PTY backend + WSL2 runtime branch)   ← pure Go (static)
GOOS=darwin  → macOS binary  (Unix PTY backend; metrics via mach/IOKit) ← cgo
GOOS=windows → Windows binary (ConPTY backend)                          ← deferred, stubs
```

Components depend on **capabilities** (`unix_pty`, `windows_conpty`, `vt_sequences`,
`linux_procfs`, …) resolved once at startup, never on raw OS-name checks. System
modules stay probe-driven: each hides itself when its source is unavailable (e.g.
`battery` on a desktop Mac with no battery).

## Deferred Work

These are explicitly outside current readiness:

- Windows/ConPTY backend;
- Agents UX and ingestion;
- diagnostics/replay harness and `doctor` / `debug-state` CLI;
- mouse actions for `RenderedBar.ClickZones`;
- richer segment shapes beyond the currently rendered flat-compatible path.

The architecture already keeps room for those later changes: typed events,
`reserved.Area` for multi-row bars, stale/error-aware module snapshots, theme
tokens, reserved `StatusState.Agents`, click zones, and config versioning.

## Where to go next

- Start with [README.md](README.md) for build, usage, and config examples.
- Use [config/config.schema.json](config/config.schema.json) and
  [internal/config/schema.go](internal/config/schema.go) as the config reference.
