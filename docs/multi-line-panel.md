# Multi-line Status Panel — Design Sketch (post-MVP)

**Status: design, not built.** Extends spec §11 (reserved area / alt-screen) and
§13.1 (bar config). The MVP fixes `bar.height = 1`; this doc records the agreed
shape for a multi-line panel so the decisions from the design discussion are not
lost. No code yet — `style`/`theme` carry the visual side when implemented.

The whole model stays "text in a fixed cell grid": no fonts, no font sizes, no
graphics layer. Everything below is colors + SGR attributes + glyph choice +
per-cell color, degrading gracefully by terminal capability.

## 1. Vertical anatomy

The reserved area, top to bottom, is three stacked zones — two optional border
lines around a static-height main panel:

```text
row = rows - reserved + 1   ┌─ top border line ──────────────────┐   toggle + style
                            │  main panel row 0                  │  ┐
        … content_rows …    │  main panel row 1                  │  ├ main panel (static N)
                            │  main panel row N-1                │  ┘
row = rows                  └─ bottom border line ───────────────┘   toggle + style
```

All three zones are the **same primitive**: *a row with a fill character + corner
caps + anchored blocks painted on top*.

- A border line is just a row whose fill is `─` with corner/edge glyphs.
- A main-panel row is a row whose fill is a space.

So "do it through styles" means a border line is a **per-row frame style**, not a
separate rendering subsystem. The top border carries blocks too (e.g. `cwd` on the
left, `git` on the right punched through the `─` fill) — borders are not pure
chrome.

When the bottom border is disabled, the panel is "open at the bottom" and the
screen edge closes it; the last panel row then sits on terminal row `rows`.

## 2. Reserved height is computed, but static

`content_rows` is the configured static parameter (the main panel). Borders add
around it:

```text
reserved = top_border(0|1) + content_rows + bottom_border(0|1)
```

Chosen model **(A)**: config asks for *content* rows; toggling a border makes the
panel taller. (Rejected **(B)**: config gives a total `height` that borders eat
into — less predictable.)

`reserved` is resolved once at startup / on resize and never changes between
keystrokes. This is deliberate: a dynamically-growing panel would re-resize the
child PTY on every change (SIGWINCH + full reflow) — dynamic height is rejected.

Everything still flows through the single source of truth:

```text
childRows = rows - reserved      (reserved.Area.ChildRows, never hardcoded)
```

## 3. Degradation (the load-bearing rule)

A static `reserved` can exceed a short terminal. The child must always keep ≥ 1
row, so:

```text
reserved = min(requested_reserved, rows - 1)
```

When the terminal is too short to honor the request, drop zones by priority:

1. drop the borders first (top, then bottom),
2. then drop main-panel rows by per-row priority (vertical analog of the existing
   horizontal block-overflow in `layout.Engine.Arrange`),
3. never go below one child row.

This implies **panel rows carry a `priority`**, like blocks do.

## 4. Scroll region & the terminal-safety invariants, generalized 1 → N

The single-row rules in [`terminal-safety.md`](terminal-safety.md) become N-row
rules. Nothing new conceptually — just "the reserved row" → "the reserved rows":

- **Scroll region:** `1 .. (rows - reserved)` in the normal screen; reset in the
  alternate screen.
- **ANSI filter:** protects / clamps the **last `reserved` rows** in the normal
  screen; does not clamp in the alt screen.
- **Alt screen:** hide all `reserved` rows, give the child full `rows`; restore on
  exit.
- **Renderer:** emits `reserved` lines; the serialized writer paints all rows in
  one synchronized-output frame, absolute-positioned, **no trailing newline** —
  the bottom row is terminal row `rows`.
- **Resize:** recompute `reserved`, re-apply the region, repaint.

## 5. Block placement (config)

Blocks gain a vertical coordinate. Two mutually-exclusive schemas (mirroring the
existing `format` XOR `[[bar.block]]`):

- **`row` per block** — structured form; `[[bar.block]]` gains `row = 0..N-1`
  alongside `anchor` (left/center/right). Plays well with per-block style and
  priority. Preferred for rich layouts.
- **`rows = ["…", "…"]`** — an array of placeholder templates, one per panel row,
  each parsed by the existing `ParseFormat` (`||` anchors). Simpler; reuses the
  parser.

Illustrative (not final), matching the §1 mock:

```toml
[bar]
content_rows = 1
[bar.border.top]    { enabled = true,  style = "rounded" }
[bar.border.bottom] { enabled = false }

[[bar.block]] { module = "cwd",  row = "top",    anchor = "left"  }
[[bar.block]] { module = "git",  row = "top",    anchor = "right" }
[[bar.block]] { module = "host", row = 0,        anchor = "left"  }
[[bar.block]] { module = "time", row = 0,        anchor = "left"  }
```

The schema already anticipates most of this: `BarConfig.Height` / `MaxHeight`
(marked *reserved, multi-line, post-MVP*), `Mode` (`single-line | agent-panel`),
and `StyleConfig.Shape` / `LeftSeparator` / `RightSeparator`. `Mode` should be a
preset layered over `content_rows`, not a second source of truth for height.

## 6. Framing details

- **Caps cost width.** Side borders `│ … │` take 2 columns; a corner cap must land
  exactly in the last column. A framed row's usable width is `cols - 2`, and its
  fill between blocks is `─` (not space).
- **Truncate inward.** Content that overflows is truncated *inside* the frame; a
  cap must never be pushed off-screen (that wraps the line → screen corruption).
- **Width sanity.** Box-drawing and braille glyphs (`╭╮│─⠋`) are width-1 in
  `runewidth`; keep a width test so framing never desyncs from `width.String`.

## 7. Depth / gradients (visual, later)

"Depth" is a vertical gradient + bevel and therefore wants ≥ 2 rows and truecolor:

- top border line slightly **lighter** than base (top-edge highlight),
- bottom border line slightly **darker** (shadow).

With only a top border you get a raised top edge; a real shadow needs the bottom
border (or an extra `▔`/`░` row). Gradients are per-cell `bg` interpolation
(24-bit), **quantized into 2–3 cell bands** to keep frame size down, degrading to
flat color on 256/16/no-color. Cheap portable depth uses shade/half-block glyphs
(`░▒▓ ▀▄`) and works without truecolor. Visuals route through `style`/`theme`;
this section is direction, not a spec.

## What stays out of scope

- **No interactive input widget.** ptyline decorates a real shell; the shell owns
  the input line. A REPL/input box "like a TUI" is a different product. Mouse /
  click handling remains post-MVP (spec §8.11).
- **No own scrollback / mouse-capture.** Native scrollback is preserved; the bar
  may scroll out of view in history (spec §10.2). Pinning it through history would
  require a custom scrollback buffer + mouse capture — explicit Non-Goals (§3).
