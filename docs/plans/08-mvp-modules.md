# 08 — MVP Modules
Status: [ ] not started
Depends on: 07
Spec refs: spec §8.7, §18; docs/state-model.md

## Goal
The MVP module set produces real values: time, hostname, static text, and cwd
(from shell integration).

## Deliverables
- `internal/modules/time.go` — strftime-style formatting.
- `internal/modules/hostname.go` — already simple; finalize.
- `internal/modules/static.go` — finalize.
- `internal/modules/cwd.go` — read `StatusState.Shell.CWD`, abbreviate `$HOME`→`~`.

## Approach
1. Time: convert the `%H:%M:%S`-style format to output (small strftime helper or a
   mapping table); refresh on `interval_ms`.
2. CWD: value sourced from `ShellState` (updated by OSC); `Refresh` reads the
   latest; tilde-abbreviate.
3. Register enabled modules from config; wire into the scheduler (plan 07).

## Invariants
Modules return typed `ModuleValue`. No per-render work (spec §8.7).

## Acceptance
- [ ] Bar (once plan 09 lands) shows current time updating each second.
- [ ] Hostname and static text render.
- [ ] `cd` in the shell updates cwd (with fish integration, plan 12).

## Tests
Time formatting table tests; cwd tilde-abbreviation; hostname error path
(`Stale`/fallback).

## Out of scope
Git (plan 15), command-duration/exit-code modules (post-MVP), custom commands.
