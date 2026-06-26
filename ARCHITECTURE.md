# Architecture

The canonical one-page map of ptyline. Deeper treatments live in [`docs/`](docs/);
this file is the index and the set of rules that bind the packages together.

## The core idea

ptyline sits between the terminal emulator and the user's shell. It runs the
shell in a pseudo-terminal that is **one (or more) rows shorter** than the real
terminal, sets the real terminal's scroll region to exclude those rows, and draws
its own status bar on them.

```text
┌──────────────────────────────┐
│ child PTY output (rows 1..n-1)│  ← child believes height = rows - reserved
│ prompt                        │
├──────────────────────────────┤
│ cwd | git | time   (row n)    │  ← ptyline's status bar (reserved area)
└──────────────────────────────┘
```

## Data flow (one direction)

The central design rule (arch.md §2): the renderer never queries git/shell/modules.
Everything flows one way into a prepared `StatusState`, which the renderer reads.

```text
Input sources → Events → Normalized state → Layout → Renderer → Terminal output

stdin ─┐
PTY  ──┤                                  ┌─ StatusState ─┐
resize ┼─► event.Bus ─► proxy.Loop ─────► │  terminal     │ ─► renderer ─► bar line
timer ─┤    (single)    (only mutator)    │  shell        │     (+ click zones)
OSC  ──┤                                  │  modules      │
signal ┘                                  └───────────────┘
```

## Components

| Package | Owns | Notes |
|---|---|---|
| `cmd/ptyline` | entrypoint | thin; calls `app.Run` |
| `internal/app` | wiring + CLI | the only place that knows how pieces fit |
| `internal/event` | `AppEvent` + `Bus` | typed, sealed event set (spec §24.1) |
| `internal/proxy` | event loop, ANSI/OSC filter, serialized writer | **only** state mutator; protects the reserved row; one writer for child output + bar frames |
| `internal/terminal` | the **real** terminal | raw mode, size, scroll region, guaranteed restore |
| `internal/pty` | the **child** PTY | spawn, resize, exit code; owns the child session/process-group + signals; Unix backend (ConPTY post-MVP) |
| `internal/status` | `StatusState`, `Module` | normalized state + typed module values |
| `internal/status/{layout,renderer,width,theme,style,icons}` | the bar UI | layout engine, display-width, theme tokens |
| `internal/modules` | built-in modules | time, hostname, user, runtime, shell, env, static, cwd, ssh, git, command |
| `internal/config` | TOML schema/loader/migrate | versioned config + nearest project `.ptyline` discovery |
| `internal/runtimeenv` | runtime profile + capabilities | detect once; components query capabilities |
| `internal/platform` | OS-specific detection | build-tagged; WSL = Linux runtime branch |
| `internal/shellintegration` | OSC contract + init scripts | cwd/exit/duration via OSC 777; **shell-agnostic — adding a shell is a template file, never Go logic** |
| `internal/reserved` | reserved-area math | single source of truth for "rows − reserved" |
| `internal/diagnostics` | health/debug state | future `doctor` / `debug-state` |

## Load-bearing invariants (read before touching terminal/pty/proxy)

These corrupt the user's terminal if broken. Full detail in
[`docs/terminal-safety.md`](docs/terminal-safety.md).

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

## Platform scope & build matrix

**MVP target: Linux and WSL/WSL2 only** (one Linux binary; WSL2 is a runtime
branch, not a separate target). Native macOS and Windows/ConPTY are **post-MVP**
(spec §4, §19) — the `darwin`/`windows` files are build-tagged stubs that must not
delay the Linux/WSL MVP.

```text
GOOS=linux   → Linux binary  (Unix PTY backend + WSL2 runtime branch)   ← MVP
GOOS=darwin  → macOS binary  (Unix PTY backend)                         ← post-MVP
GOOS=windows → Windows binary (ConPTY backend)                          ← post-MVP
```

Components depend on **capabilities** (`unix_pty`, `windows_conpty`, `vt_sequences`,
`linux_procfs`, …) resolved once at startup, never on raw OS-name checks.

## Future-proofing already wired in

So post-MVP features (arch.md) don't force a redesign, the scaffold already
includes: the typed event bus, the `reserved.Area` abstraction (multi-line bars),
typed `ModuleValue`/`ModuleSnapshot` (stale/error aware), theme tokens, reserved
`StatusState.Agents`, `RenderedBar.ClickZones` (mouse), and `config_version`.

## Where to go next

- New here? Read [`docs/ARCHITECTURE-overview.md`](docs/ARCHITECTURE-overview.md).
- Implementing? Follow [`docs/plans/`](docs/plans/) in order from `00`.
- Multi-line panel (post-MVP design): [`docs/multi-line-panel.md`](docs/multi-line-panel.md).
