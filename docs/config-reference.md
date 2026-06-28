# Configuration Reference

Source: `internal/config`. Design: spec §13, ARCHITECTURE.md §17.

## Location

```text
$XDG_CONFIG_HOME/ptyline/config.toml      (preferred)
~/.config/ptyline/config.toml             (fallback)
```

Override with `ptyline --config <path>`. A missing file is not an error: the
built-in `config.Default()` is used (spec §13).

## Project-local `.ptyline`

After a shell reports a `cwd` change, ptyline searches that directory and its
parents for the nearest `.ptyline` TOML file. Project config is a visual/profile
overlay on top of the main config and the optional CLI-selected overlay: it may
adjust presentation, but it must not choose child commands or define custom
command modules.

```toml
config_version = 1

[bar]
format = "{cwd} || {git} || {time}"

[module.env]
env = ["APP_ENV", "REGION"]
```

See `docs/features/project-bar-overlays.md` for the planned `--ptyline` overlay
and project `.ptyline` contract.

## Format & versioning

TOML, not Markdown (spec §13). `config_version` is **required** and currently must
equal `1`. The loader flow is `read → migrateToLatest → parse → validate → apply`;
config is migrated forward before parsing so old files keep working (spec §13.1,
§24.6). Add migration steps in `internal/config/migrate.go`.

## MVP example (spec §13)

The MVP default uses only initial modules (`time`, `cwd`, `hostname`, static) —
git is a post-MVP provider. The bar is always hidden in the alternate screen
(spec §11, §13.1).

```toml
config_version = 1
shell = "auto"            # auto uses $SHELL or platform default; a command/path also works
refresh_interval_ms = 1000

[bar]
format = "{hostname} {cwd} || {time}"   # left = before "||", right = after
justify = "center"
separator = " | "

[module.time]
enabled = true
format = "%H:%M:%S"
interval_ms = 1000

[module.cwd]
enabled = true
mode = "shell-integration"   # shows fallback (empty) until an adapter supplies cwd

[module.hostname]
enabled = true
```

## MVP configuration contract (spec §13.1)

- `config_version` is required and must equal `1`.
- Unknown top-level keys, unknown module keys, invalid enum values, and invalid
  width expressions are **startup errors** that name the file and key. Config is
  loaded → migrated → validated → applied; invalid config is never silently
  reinterpreted.
- `bar.format` splits on `||` into sections: one section is left, two are
  left/right, three are left/center/right.
- `|` inside `bar.format` or `[[bar.row]].format` is a separator marker. It draws
  the active row separator (`[[bar.row]].separator`, falling back to
  `bar.separator`) and collapses when either neighboring block in that section is
  empty. Use `\|` for a literal pipe.
- `bar.justify` controls center-section placement: `center` places it in the
  free space between left/right; `absolute_center` uses the full bar center when
  it fits; `left` and `right` pin it next to the corresponding side section.
- `bar.min_block_width` (default `0` = disabled): a module block whose allocated
  cell width falls below this threshold is hidden entirely instead of being
  truncated to a tiny sliver. Literal blocks (separators, spacing) are never
  hidden by this rule. Set e.g. `min_block_width = 5` to keep the bar readable on
  narrow terminals.
- Module IDs are unique; a placeholder references an enabled module by ID. Defaults are
  applied only for omitted values. An unavailable optional provider renders its
  fallback and emits a diagnostic — it never blocks terminal I/O.

## Format placeholders

Format strings use a small grammar:

```text
{module}  module placeholder
||        section split: left / center / right; not drawn
|         separator marker; draws the active separator
\|        literal pipe
```

For example:

```toml
[[bar.row]]
separator = " : "
format = "{env} | {runtime} | {shell}"
```

renders as `env : runtime : shell`. If `{env}` is empty, it renders as
`runtime : shell` with no dangling separator.

Placeholders may carry a small width/alignment suffix:

```toml
[bar]
format = "{cwd:<30} || {git:^20} || {time:>8}"
justify = "absolute_center"
```

The supported suffixes are intentionally small:

```text
{name}      auto width, left aligned
{name:<30}  30 terminal cells, left aligned
{name:^20}  20 terminal cells, centered
{name:>8}   8 terminal cells, right aligned
```

Widths are terminal display cells, not bytes.

## Theme, icons, style (spec §8.9, §8.10; ARCHITECTURE.md §16)

```toml
[theme]
color_scheme = "default"   # default, or themes/<name>.toml next to config.toml
style = "flat"             # minimal|flat|powerline|box|pill
icons = "ascii"            # nerd-font|emoji|ascii
fallback = "ascii"

[theme.palette]            # inline palette overrides
"base.bg" = "#1e1e2e"
"base.fg" = "#cdd6f4"
accent = "#89b4fa"

[theme.status]             # semantic tokens (ARCHITECTURE.md §16)
ok = "green"
warn = "yellow"
error = "red"

[icons]
enabled = true
preset = "ascii"
emoji_width = "auto"       # auto|1|2
fallback = true

[style.time]
fg = "white"
bg = "blue"
animation = "pulse"        # visual effect when module.<id>.animation = true
bold = true
left_cap = "["
right_cap = "]"
padding_left = 1
padding_right = 1
```

If `color_scheme = "catppuccin"`, ptyline loads
`themes/catppuccin.toml` from the same directory as the effective config file.
Theme files use the same `[palette]` and `[style.<id>]` shape; see
`docs/features/file-themes.md`.

## Custom command module (spec §8.7, §17)

```toml
[bar]
format = "{hostname} {cwd} {kube} || {time}"

[module.kube]
source = "exec" # optional for non-built-in module IDs; unknown IDs default to exec
command = "kubectl config current-context"
interval_ms = 10000
timeout_ms = 200
format = "{stdout}"
refresh_on_command = ["kubectl config use-context"]
```

Module formats that support multiple placeholders can use `|` as a conditional
separator marker and `separator` to choose the rendered text:

```toml
format = "{stdout} | {stderr} | {exit_code}"
separator = "•"
```

Empty fields drop adjacent separators.

Custom commands run **locally** with a timeout; config is trusted user input but
commands must always be time-bounded (spec §16, §17).
`refresh_on_command` triggers an immediate refresh after a matching foreground
command exits. Matching normalizes whitespace and accepts exact or prefix matches
with a space boundary, so `"gh auth login"` also matches `"gh auth login --web"`.

`source = "time"` can reuse a built-in provider under a custom placeholder:

```toml
[module.time_utc]
source = "time"
format = "%H:%M UTC"
```

## Field map

The Go schema lives in `internal/config/schema.go` (`Config`, `BarConfig`,
`RowConfig`, `ModuleConfig`, `ThemeConfig`, `IconsConfig`, `StyleConfig`).
Defaults in `default.go`.
