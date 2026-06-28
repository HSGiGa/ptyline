# 09 — Layout & Renderer
Status: [x] done
Depends on: 07, 08
Spec refs: spec §8.6, §8.8; ARCHITECTURE.md §7, §8, §15; docs/layout-and-rendering.md, docs/terminal-safety.md

## Goal
A real one-line bar pinned to the last row: blocks parsed from config/format,
arranged into left/center/right with width resolution and priority overflow,
measured by display width, drawn with absolute positioning. **Milestone: visible
bar.**

## Deliverables
- `internal/status/width/width.go` — `mattn/go-runewidth` + truncation.
- `internal/status/layout/layout.go` — three-section packing, fill/percent, min/max,
  priority overflow.
- `internal/status/renderer/renderer.go` — assemble the line + click zones.
- Drawing code (in `app`/`proxy`): save → move to bar row → clear → write → reset →
  restore; skip if unchanged.

## Approach
1. Add `go-runewidth`; `make tidy`. Implement `width.String`/`Truncate`.
2. Parse the `format` string (and/or `[[bar.block]]`) into `[]layout.Block`.
3. `Engine.Arrange`: allocate fixed/percent/fill widths within `barWidth`, clamp
   min/max, drop lowest-priority blocks on overflow.
4. Renderer renders each block's snapshot value, applies style/icon, truncates to
   its area, joins with separators, pads to width.
5. Draw with absolute positioning; **never** a trailing newline (spec §8.6).

## Invariants
Measure display width, not bytes/runes (spec §8.6). No newline after the bar. Skip
redraw when the line is byte-identical (spec §16). Renderer reads state only.

## Acceptance
- [x] Left/center/right blocks render in the expected positions (spec §20.13).
- [x] Long output scrolls only above the bar; bar stays on the last row (§20.3–§20.5).
- [x] Narrow terminals drop low-priority blocks instead of overflowing.

## Tests
Width/truncation table tests; layout packing (fill/percent/overflow) tests;
renderer golden-line tests for sample states, including border-row fill and
empty-module hiding.

## Out of scope
Color/theme/icons depth (plan 10) — render plain or basic fg/bg here. Mouse
handling (post-MVP).
