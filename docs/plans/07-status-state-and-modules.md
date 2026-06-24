# 07 — Status State & Module Framework
Status: [x] done
Depends on: 05, 06
Spec refs: spec §8.5, §8.7; arch.md §3, §9; docs/state-model.md

## Goal
A `StatusState` updated solely by the loop, plus a module runner that refreshes
modules on their own intervals (with timeouts) and writes cached snapshots —
without ever doing work during render.

## Deliverables
- `internal/status/state.go` — state-update helpers (apply `ShellMeta`, resize,
  module snapshot).
- A module scheduler (in `status` or `app`): per-module timer → `Refresh(ctx)` with
  timeout → `ModuleUpdated` event.
- Loop handlers for `ShellMeta`, `ModuleUpdated`, `Resize`, `Tick`.

## Approach
1. Map OSC keys (`cwd`/`exit_code`/`duration_ms`/`command`) to `ShellState`.
2. Scheduler: for each module with `Interval()>0`, run a ticker goroutine that
   calls `Refresh` under a `context.WithTimeout`; send the snapshot as an event.
3. On timeout/error, mark the snapshot `Stale`/`Error` (don't block).

## Invariants
Renderer reads cached snapshots only (spec §8.7). Only the loop mutates state.
Expensive refreshes are time-bounded (spec §16).

## Acceptance
- [x] `ShellMeta` updates `StatusState.Shell` and the next redraw reflects it.
- [x] A slow module times out and is marked stale without stalling the bar.

## Tests
State-apply unit tests (OSC → ShellState). Scheduler test with a fake slow module
asserting timeout → stale snapshot.

## Out of scope
Specific modules (plan 08), rendering (plan 09).
