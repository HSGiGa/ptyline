# Architecture Overview

A longer companion to the root [`ARCHITECTURE.md`](../ARCHITECTURE.md). Read that
first for the component table and invariants; this file narrates the data flow and
the reasoning behind the structure.

## The problem

A normal interactive shell owns the whole terminal grid. We want a persistent
status line that survives scrolling and full-screen apps, **without** building a
terminal multiplexer. tmux solves this by emulating terminals inside panes;
ptyline instead does the minimum: shrink the child's view by one row and protect
that row.

Two mechanisms combine:

1. **Sizing** — the child PTY is told it is `rows - reserved` tall, so most
   programs simply never draw on the reserved rows.
2. **Scroll region** — `CSI 1 ; (rows-reserved) r` confines scrolling to the
   shell area, so output scrolls *under* a stationary bar.

Neither is sufficient alone (a program can still emit absolute cursor moves or
reset the scroll region), so a lightweight **ANSI/OSC filter** sits in the
child→terminal path to clamp anything that would touch the reserved rows.

## Why provider → event → state → render

The naive design renders by querying git/shell/clock inline. That couples
rendering to slow I/O, makes the bar janky, and is untestable. ptyline instead
treats the bar as a **view of structured runtime state** (arch.md §2):

- **Providers** (modules, shell-integration OSC, future sockets) collect data on
  their own schedule and push snapshots.
- The **event bus** normalizes every input into an `AppEvent`.
- The **event loop** is the single writer of state and the single driver of
  output — everything else is pure.
- The **renderer** reads a prepared `StatusState` and produces a line. It is a
  pure function of state, so it is trivially testable and fast.

This separation is what lets agents, multi-line panels, mouse zones, and socket
providers be added later by introducing a new provider/event, not by rewriting
the core.

## Lifecycle (spec §7)

```text
read base config → detect runtime → save terminal state → raw mode → query size
→ set scroll region 1..rows-reserved → spawn child PTY (cols × rows-reserved)
→ run event loop (proxy IO, render bar, handle resize/signals/ticks)
→ child exits → restore terminal state → exit with child code
```

## Project-local bar configuration

On each shell-integration `cwd` event, ptyline searches the current directory and
its parents for the nearest `.ptyline` TOML file. When found, its validated
`bar.format` replaces the base bar format; when no file is found, ptyline returns
to the base configuration. This is presentation-only at runtime: a project file
does not change the child command or execute a custom command.

The restore step is guaranteed by `defer` and signal handling: if anything after
"save terminal state" fails, the terminal is put back first (spec §15).

## Concurrency model

A single event loop (Go `select` over the bus channel) owns all mutable state.
Producers run in their own goroutines and only *send* events:

- stdin reader → `StdinInput`
- PTY reader → `PtyOutput`
- `SIGWINCH` handler → `Resize`
- `SIGINT`/`SIGTERM` handler → `TerminationSignal`
- status ticker → `Tick`
- module refreshers → `ModuleUpdated`

Because only the loop mutates state, there are no locks on the hot path and the
renderer always sees a consistent snapshot. See [`event-bus.md`](event-bus.md).
