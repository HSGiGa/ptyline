# 14 — Diagnostics & Replay Test Harness
Status: [ ] not started
Depends on: 05, 06
Spec refs: spec §20, §20.1 (verification matrix); arch.md §18, §19; docs/testing-and-replay.md

## Goal
A deterministic record/replay harness for terminal behavior, plus a diagnostics
layer that subsystems report into — enabling regression tests for the safety
invariants and future `doctor`/`replay` commands.

## Deliverables
- A replay package (e.g. `internal/replay`): recording format + a driver that
  feeds recorded events into the event loop and captures terminal output.
- `internal/diagnostics` reporting wired from filter/modules/render.
- Recorded fixtures for vim/less/htop/ssh sessions.

## Approach
1. Define the recording (ordered timed events: stdin, PTY output, resize, tick,
   OSC, child exit, signal).
2. Replace live producers with a recording source; inject time/`Tick`.
3. Capture stdout and assert: reserved row untouched, scroll region correct,
   redraw-on-change, OSC consumed, terminal restored.
4. Report into `diagnostics.Record` (last module error, render duration, ANSI
   warnings).

## Invariants
Replays are fully deterministic (no real clock or goroutine timing).

## Acceptance — the §20.1 verification matrix
Deterministic tests + fixtures must cover every row of spec §20.1:
- [ ] Normal shell output: byte order retained; bottom row redrawn without a newline.
- [ ] Scroll-margin reset from child: clamped to `rows-1` in the normal screen.
- [ ] Split CSI/OSC input: same result regardless of PTY read boundaries.
- [ ] Alt-screen enter/exit: child receives `rows`, then `rows-1`; bar absent, then restored.
- [ ] Resize: correct PTY size + margin for the current screen mode.
- [ ] Controlled shutdown: child process group reaped; modes/cursor/margins restored.
- [ ] Diagnostics capture an injected module error.

## Tests
The harness *is* the test infrastructure; add fixtures incrementally and back-fill
plans 06/13 assertions onto it.

## Out of scope
`ptyline doctor`/`debug-state`/`replay` CLI commands (post-MVP; the harness
underpins them).
