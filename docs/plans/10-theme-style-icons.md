# 10 — Theme, Style & Icons
Status: [x] done
Depends on: 09
Spec refs: spec §8.9, §8.10; ARCHITECTURE.md §14, §16; docs/layout-and-rendering.md

## Goal
Semantic theme tokens resolve to escape sequences for the detected terminal
capabilities; blocks get fg/bg/attributes/padding/separators; icons render via
nerd-font/emoji/ascii with fallback. One readable default theme.

## Deliverables
- `internal/status/theme/theme.go` — default palette + token→escape; no-color mode.
- `internal/status/style/style.go` — `Apply` emits SGR + padding + separators per
  shape (start with `flat`).
- `internal/status/icons/icons.go` — preset selection + fallback; emoji width policy.
- Terminal-capability detection in `runtimeenv` (truecolor/nerd_font/emoji) via
  `muesli/termenv`.

## Approach
1. Add `muesli/termenv`; `make tidy`. Probe truecolor/256/no-color, set caps.
2. Build the default token map (`ok/warn/error/muted/accent`, agent.* reserved).
3. `Style.Apply`: resolve fg/bg via theme, add attributes, padding, separators,
   reset at the end. Implement `flat` now; leave `powerline/pill/box` stubs.
4. Icons: ASCII fallback always usable (spec §20.15); honor `emoji_width`.

## Invariants
Modules never write raw ANSI — they go through tokens (ARCHITECTURE.md §16). The default
theme is readable without Nerd Font or emoji (spec §20.15). Styling must not
corrupt child output (spec §20.14) — always reset.

## Acceptance
- [x] `NO_COLOR`/no-color terminals render plain, correct text.
- [x] Basic fg/bg styling renders without leaking escapes into the shell area.
- [x] ASCII preset is fully usable.

## Implementation note
Color capability is detected from the environment (`NO_COLOR`/`COLORTERM`/`TERM`)
in `runtimeenv.detectColor` rather than via `termenv` — Nerd-Font/emoji can't be
detected anyway (config-driven), so no external dep was added. The renderer
(`internal/status/renderer`) now routes every segment through `style.Apply` +
`theme`; no raw ANSI remains. Default palette is Catppuccin-Mocha-derived.

## Tests
Token resolution (color vs no-color); `Style.Apply` golden sequences; icon
fallback selection.

## Out of scope
Powerline/pill/box shapes and color-scheme presets (gruvbox/catppuccin/…) —
post-MVP (spec §19).
