# Section Separator

Status: planned

## Goal

Make `||` in the bar format render a visible separator character at each section
boundary, using the `bar.separator` value. This removes the need to embed literal
separator text in the format string and gives one place to change the divider
across all rows.

## User Model

Set the separator once in `[bar]` and write `||` as usual:

```toml
[bar]
separator = " | "
format = "{identity} || {runtime} {shell} || {gh} {time}"
```

Rendered (80 cells):

```
viachaslau@host ~/proj | macos fish | HSGiGa 14:32:00
```

The separator is inserted as a literal block at each `||` boundary. It inherits
no module style — it is plain bar-color text. To style it, add `[style.separator]`
and the renderer applies it to every injected separator block.

Setting `separator = ""` (or omitting it) produces no visible divider; `||`
remains a pure layout directive. This is the default.

## Empty Section Suppression

When adjacent sections produce no visible output (e.g. a module is hidden or
empty), the separator between them is suppressed:

```
{command} || || {git}
```

If `command` is empty and `git` is empty, no separators are rendered.
If only `command` is empty, the leading separator is suppressed so the result
starts cleanly with `git`'s value.

Rule: a separator is rendered only when at least one of its two adjacent blocks
is visible and non-empty.

## Fill Rows

Separator injection is disabled on rows that set `fill`. A fill row draws a
ruled line (`─ ─ ─`) across the bar; inserting a separator glyph into a fill
character run is undefined and unwanted.

## Interaction With Inline Literal Text

If the format string already contains literal text at a section boundary, the
injected separator is additive:

```toml
format = "{identity}  || {time}"   # two spaces before ||
```

Results in `identity  | time` (two spaces + separator). Avoid mixing both
styles; use one or the other.

## Config Reference

```toml
[bar]
separator = " | "      # string rendered at each || boundary; "" = no separator
```

The `[style.separator]` key (optional) styles only the injected separator
blocks:

```toml
[style.separator]
fg  = "muted"
dim = true
```

## Implementation Sketch

`ParseFormat` currently discards `||` after splitting. The change:

1. After splitting on `||`, for each boundary between non-empty sections insert
   a `literalBlock(cfg.Bar.Separator, adjacentAnchor)` with `Priority = 1`
   (same as other literals, dropped last under overflow).
2. If `cfg.Bar.Separator == ""`, skip injection entirely — zero behavior change.
3. At render time, apply `style.separator` to those blocks if the style is
   defined; otherwise render plain.
4. Fill-row detection (`RowConfig.Fill != ""`) skips injection for that row.

The renderer already handles literal blocks with per-block styles, so no
renderer changes are needed beyond plumbing the style lookup.

## Non-Goals

- No per-`||` separator override in the format string (use inline literals for
  that).
- No automatic separator between every pair of blocks within a section; this
  feature is section-boundary only.
- No separator in `[[bar.block]]` structured layout (blocks already carry
  `left_separator` / `right_separator` in their style).
