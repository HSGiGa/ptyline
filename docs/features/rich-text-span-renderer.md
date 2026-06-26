# Rich Text / Span Renderer

Status: proposed

## Goal

Allow one module to return multiple styled text spans instead of one plain text
value. This lets a stateful module such as `command` style and animate its
semantic parts independently:

```toml
[module.command]
enabled = true
format = "{active} {last} {exit} {duration}"
max_width = 60

[module.command.active]
animation = "glint"
fg = "#f2b35d"

[module.command.last]
fg = "base.fg"

[module.command.exit]
ok_fg = "ok"
error_fg = "error"

[module.command.duration]
fg = "muted"
```

The user-facing model is still one `{command}` block in the bar. The nested
tables configure the placeholders inside that module, not separate status-bar
modules.

## User Model

`[module.command]` owns the command lifecycle:

- while a command runs, `{active}` is populated and `{last}`, `{exit}`,
  `{duration}` are empty;
- after the command finishes, `{active}` is empty and `{last}`, `{exit}`,
  `{duration}` are populated;
- in idle/no-history state, all placeholders may be empty.

After placeholder expansion, the module should collapse redundant whitespace so a
single format works across states:

```toml
format = "{active} {last} {exit} {duration}"
```

Example output:

```text
npm test
git pull ok 1.2s
npm test exit 2 8.4s
```

## Data Model

Today `ModuleValue` can carry plain text/status/JSON values. Rich rendering needs
a span value that preserves display text separately from style metadata:

```text
ModuleValue.Kind = KindSpans

Span {
  Text       string
  Role       string // active | last | exit | duration | ...
  StyleID    string // optional resolved style hook
  Level      StatusLevel // optional ok/warn/error semantic level
  Animation  string // optional animation mode for this span
}
```

The renderer must continue to make all layout decisions from visible cell widths,
not ANSI byte lengths. ANSI escapes are emitted only after layout and truncation
are resolved.

## Configuration

Nested tables under a module configure semantic placeholders:

```toml
[module.command.active]
animation = "glint"
fg = "#f2b35d"
bold = true

[module.command.exit]
ok_text = "ok"
error_format = "exit {code}"
ok_fg = "ok"
error_fg = "error"
```

These tables are module-specific. They are not global `[style.*]` blocks because
they may contain domain settings such as `ok_text`, `error_format`, or
`animation` rules tied to command state.

Global styles still apply at the outer block level:

```toml
[style.command]
bg = "base.bg"
padding_left = 1
padding_right = 1
```

The effective style is:

```text
outer block style + module placeholder style + semantic level override
```

## Rendering Rules

- Spans must never include raw ANSI from providers.
- Empty spans are removed before layout.
- Whitespace between removed spans is normalized by the module or a shared
  formatter helper.
- Truncation applies to the whole module block first; span boundaries should be
  preserved where possible.
- Color-only animations such as `glint`, `pulse`, and `blink` may apply to a
  single span without changing display width.
- In no-color mode, rich spans degrade to plain text.

## Command Module Target

The first user of spans should be the command lifecycle module:

```toml
[module.command]
enabled = true
format = "{active} {last} {exit} {duration}"
max_width = 60

[module.command.active]
animation = "glint"
fg = "#f2b35d"

[module.command.exit]
ok_fg = "ok"
error_fg = "error"

[module.command.duration]
fg = "muted"
```

The module produces:

- active state: one `active` span;
- completed success: `last`, `exit`, and `duration` spans;
- completed failure: same spans, with `exit` at error level;
- idle/no-history: empty value.

## Non-Goals

- No raw ANSI passthrough from modules.
- No per-render command execution.
- No general HTML/CSS-like markup language.
- No making `{active}`, `{last}`, `{exit}`, or `{duration}` independent top-level
  modules by default.

## Acceptance

- A module can return spans and render them with the same visible width as the
  equivalent plain text.
- Per-span colors do not corrupt layout, padding, truncation, or alignment.
- `{command}` can animate only its active span while leaving completed command
  output static.
- No-color mode renders readable plain text.
- Existing plain-text modules continue to render unchanged.
