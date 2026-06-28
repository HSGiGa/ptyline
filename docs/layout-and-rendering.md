# Layout & Rendering

Source: `internal/status/{layout,renderer,width,theme,style,icons}`. Design: spec
§8.6–§8.10, ARCHITECTURE.md §7, §8, §15, §16.

## Three concerns, kept separate (spec §8.8)

```text
layout  → where a block goes and how much space it gets
content → what a block renders (a module value)
style   → how it looks (colors, attributes, shape, padding)
```

## The layout engine (not string concatenation)

Even though the MVP config exposes a one-line `format` string, it is parsed into
**blocks** with metadata, not treated as a final raw string (ARCHITECTURE.md §7). A block:

```text
anchor    left | center | right     (which terminal side)
align     left | center | right     (within its area)
justify   left | center | right | absolute_center
width     auto | fill | N | N%      (measured in TERMINAL CELLS)
min_width / max_width
truncate  left | right | middle | none
priority  number                    (for overflow)
style     style id
```

The engine packs the three sections (left/center/right), resolves `fill`/percent
widths, clamps to min/max, and assigns each visible block a `[startCol,endCol)`
range on the bar row. `justify` is a bar-level policy for the center section:
`center` is relative to the free space between left/right; `absolute_center` pins
the center section to the full row's geometric center when it does not overlap.
`align` remains local to a block's allocated width.

The one-line format parser supports section and separator markers plus a compact
width/alignment suffix:

```text
{identity} || {env} | {runtime} | {shell} || {gh} | {time}
```

`||` splits left/center/right sections and is not drawn. A single `|` marks a
block separator; the renderer draws the active row separator at that position and
collapses it when either neighboring block in the same section is empty. Use
`\|` for a literal pipe.

Width/alignment suffixes attach to placeholders:

```text
{cwd:<30} || {git:^20} || {time:>8}
```

## Display width is mandatory (spec §8.6, §8.10)

All measurement uses `status/width` (display cells), **never** byte length or rune
count. CJK and many emoji are 2 cells; combining marks are 0. Emoji width is
ambiguous across terminals, so the config offers a conservative width policy
(`icons.emoji_width = auto|1|2`).

## Priority-based overflow (ARCHITECTURE.md §8)

When the terminal is narrow, low-priority blocks are dropped or rendered in a
compact variant rather than letting the bar overflow:

```text
full:    ~/project main ✓ | 🤖 reviewer running 4m · tester blocked | 18:42
medium:  project main ✓ | 🤖 2 agents | 18:42
compact: 🤖2 | 18:42
```

This matters most for future agent info, whose text can grow without bound.

## Theme tokens, not raw ANSI (ARCHITECTURE.md §16)

Modules/blocks request semantic tokens (`ok`, `warn`, `error`, `accent`,
`agent.running`, …). The theme translates a token to an escape sequence for the
current capabilities (truecolor / 256 / no-color). This is what enables presets,
light/dark, no-color, and accessibility modes without touching module code.

## Styles are terminal text, not a GUI (spec §8.9)

Segment "shapes" (`flat`, `powerline`, `pill`, `box`) are produced with Unicode
glyphs, background colors, caps, and padding. Nerd-Font/Powerline glyphs are
optional; an ASCII fallback must always be usable (spec §20). Icons come from
`status/icons` with a primary glyph + ASCII fallback (spec §8.10).

## RenderedBar and click zones (ARCHITECTURE.md §15)

`renderer.Render` returns `RenderedBar{ Line, ClickZones }`. Click zones map cell
ranges to actions and are **ignored until mouse support is enabled** (post-MVP).
Returning them now means mouse support is additive, not a rewrite.

## Drawing

The caller draws `Line` with absolute positioning and no trailing newline — see
[`terminal-safety.md`](terminal-safety.md) §4. Skip the redraw when `Line` is
unchanged (spec §16).
