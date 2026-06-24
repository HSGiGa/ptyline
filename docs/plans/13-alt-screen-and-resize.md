# 13 — Alternate Screen & Resize Hardening
Status: [ ] not started
Depends on: 06, 09
Spec refs: spec §11, §12, §10, §8.3; docs/terminal-safety.md

## Goal
Full-screen apps and resizes are handled per the fixed MVP policy: **the bar is
hidden in the alternate screen and the child gets full height**, and the bar +
scroll region are correctly restored on exit and across resizes in both modes.
**Completes the MVP (spec §18).**

## Deliverables
- Serialized-writer driven alt-screen entry/exit procedures (spec §11).
- Mode-specific debounced resize path (spec §12).

## Approach
1. **Alt-screen entry** (filter signals it): stop scheduled redraws → reset scroll
   region → forward the enter sequence → resize child PTY to `cols × rows` (full).
2. **Alt-screen exit**: forward the leave sequence → resize child to
   `cols × max(rows-1,1)` → set scroll region `1..rows-1` (when ≥ 2 rows) → redraw.
3. **Resize** (debounce ~30–50ms): re-query size; if alt active → child `cols × rows`,
   region stays reset, skip bar; else → child `rows-1`, region `1..rows-1`, redraw.
4. Guard tiny terminals (< 2 rows): clamp child rows to 1; skip the bar.
5. All terminal writes go through the one serialized writer; never interleave a bar
   frame with child output (spec §8.3).

## Invariants
**Read docs/terminal-safety.md §2–§4a.** No clamping in the alt screen; bar frames
suppressed while alt active; scroll region/child size reapplied on every transition
and resize; reserved row never written by child output in the normal screen.

## Acceptance
- [ ] Entering vim/htop hides the bar and gives the child full height; exiting
  restores the bar, scroll region, and reserved row (spec §20.6, §20.7).
- [ ] Resize keeps correct PTY size + margins for the current mode (spec §20.5).
- [ ] Very small terminals don't crash or corrupt (spec §15).

## Tests
Replay tests (plan 14) for alt-screen enter/exit and resize in both modes; assert
child receives `rows` then `rows-1`, bar absent then restored (spec §20.1 matrix).

## Out of scope
Optional *visible* bar in alt screen, multi-line/panel mode (post-MVP, spec §19).
