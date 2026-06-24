# 03 — Terminal Controller
Status: [x] done
Depends on: 00
Spec refs: spec §8.1, §6, §15; docs/terminal-safety.md

## Goal
Full control of the real terminal: raw mode, size query, scroll-region set/reset,
cursor ops, and **guaranteed** restoration on every exit path.

## Deliverables
- `internal/terminal/raw_mode.go` — real `term.MakeRaw`/`term.Restore`.
- `internal/terminal/size.go` — real `term.GetSize` (TIOCGWINSZ).
- `internal/terminal/{controller,scroll_region}.go` — finished behavior.

## Approach
1. Add `golang.org/x/term`; `make tidy`.
2. `enableRaw`/`disableRaw` store and restore `*term.State` on the tty fd.
3. `QuerySize` from the tty fd.
4. `Restore` performs the exact cleanup order (reset region → reset attrs →
   restore cursor → restore mode → show cursor) and is idempotent.
5. Buffer control writes; handle short writes and `EINTR`.

## Invariants
**Read docs/terminal-safety.md.** Restore must run on normal exit, signals, child
exit, and init failure after state was modified. `Restore` safe to call twice and
from a signal handler.

## Acceptance
- [ ] Entering raw mode and restoring leaves the terminal exactly as before.
- [x] Scroll region set to `1..childRows`; reset clears it.
- [ ] A forced panic still restores the terminal (via `defer`).

## Tests
Unit-test `SetScrollRegion`/`CursorTo` byte output. Manual/integration: verify no
corruption after exit. (Raw-mode tests need a pty — combine with plan 14.)

## Out of scope
Signal wiring and the loop (plan 05). ConPTY specifics (handled in pty, plan 04).
