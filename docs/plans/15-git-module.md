# 15 — Git Module
Status: [x] done
Depends on: 07, 09
Spec refs: spec §8.7, §16, §19; docs/state-model.md

## Goal
The canonical expensive module: show the current branch, refreshed on its own
interval with a timeout and fully cached — proving the caching abstraction.

## Deliverables
- `internal/modules/git.go` — real `Refresh` using the cwd-aware git invocation.

## Approach
1. `Refresh(ctx)`: run `git rev-parse --abbrev-ref HEAD` (and later `git status
   --porcelain` for dirty state) under `ctx`'s timeout, in the shell's cwd
   (`StatusState.Shell.CWD`).
2. On non-repo: empty/hidden value. On timeout: keep the previous value, mark
   `Stale`. On error: `Error` set.
3. Default interval 2000ms, timeout 100ms (spec §16).

## Invariants
**Never run git per redraw** (spec §8.7) — only on the interval. Always
time-bounded (spec §16). Renderer reads the cached snapshot.

## Acceptance
- [x] Branch shows and updates within one interval after `git checkout`.
- [x] A hung git (simulated) yields a stale snapshot, never a stalled bar.
- [x] Outside a repo, the module hides cleanly.

## Tests
`internal/modules/git_test.go` runs against a temp git repo (`init`, commit,
checkout) and asserts the rendered branch value. A fake hung git binary asserts
timeout → stale.

## Implementation note
Git still refreshes on its own interval as a fallback, but `cwd` shell metadata
now also triggers an immediate async git refresh. Empty git values hide the block
in normal status rows; border rows fill the empty section with the row's `Fill`
character.

## Out of scope
Dirty-state details, ahead/behind counts (post-MVP polish, spec §19).
