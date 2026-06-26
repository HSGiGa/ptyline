# File-Based Themes

Status: implemented

## Goal

Keep one readable built-in default theme in code and allow additional user themes
to be loaded from TOML files.

The built-in theme is the stable fallback: ptyline must render correctly when no
theme files exist, when the config file is missing, or when the terminal supports
only reduced color. File themes extend that default without moving ANSI rendering
or terminal capability handling into user data.

## User Model

The main config selects a theme by name:

```toml
[theme]
color_scheme = "catppuccin"
```

Theme name resolution:

- empty or `"default"` uses the built-in `theme.Default(mode)`;
- any other name loads `themes/<name>.toml`;
- an explicitly selected missing or invalid theme is a startup error.

Theme files are TOML. The extension should be `.toml`, not a custom `.them`
extension, so editors, linters, and tests can treat the file as ordinary TOML.

## Search Paths

The initial implementation resolves theme files relative to the effective main
config file:

```text
<directory containing config.toml>/themes/<name>.toml
```

With the default config path, this is:

```text
$XDG_CONFIG_HOME/ptyline/themes/<name>.toml
~/.config/ptyline/themes/<name>.toml
```

Project-local theme search can be added later if project config becomes
responsible for visual overrides. The built-in default is not a file and is
always available.

## Theme File Format

A theme file should be data-only. It defines palette tokens and optional style
defaults; it must not contain raw ANSI escape sequences.

```toml
name = "catppuccin"

[palette]
"base.bg" = "#1e1e2e"
"base.fg" = "#cdd6f4"
accent = "#89b4fa"
ok = "#a6e3a1"
warn = "#f9e2af"
error = "#f38ba8"
muted = "#9399b2"

[style.time]
fg = "base.fg"
bg = "accent"
bold = true
padding_left = 1
padding_right = 1
```

Palette values may be:

- hex colors: `#rrggbb`;
- named ANSI colors already supported by `internal/status/theme`;
- references to other palette tokens, including tokens defined in the same file.

The theme resolver continues to own conversion to SGR for `truecolor`, 256-color,
16-color, and no-color terminals.

## Merge Rules

Theme assembly should be layered in this order:

1. Built-in default palette and style defaults.
2. Loaded `themes/<name>.toml`, if `color_scheme` is not `default`.
3. Inline overrides from the main config, such as `[theme.palette]`,
   `[theme.status]`, `[theme.agent]`, and `[style.<id>]`.

This keeps the default theme complete while allowing small theme files and
per-machine overrides.

## Validation

Theme loading should reject:

- unknown top-level keys;
- invalid color values;
- style fields that do not match the config schema;
- a `name` field that does not match the selected file name, if `name` is set.

Validation errors should include the theme file path and the offending key.

## Non-Goals

- No custom `.them` parser.
- No raw ANSI in theme files.
- No dependency on Nerd Font or emoji availability.
- No requirement to ship multiple built-in color schemes in the MVP.

## Relationship To Existing Plans

Plan 10 (`docs/plans/10-theme-style-icons.md`) already defines one readable
default theme as the completed MVP. File-based themes are a post-MVP extension
for presets such as gruvbox, catppuccin, nord, and solarized.
