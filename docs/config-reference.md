# Configuration Reference

Source: `internal/config`. Design: spec §13, arch.md §17.

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
git is a post-MVP provider. There is no `show_in_alternate_screen`: the bar is
always hidden in the alternate screen in the MVP (spec §11, §13.1).

```toml
config_version = 1
shell = "auto"            # auto uses $SHELL or platform default; a command/path also works
refresh_interval_ms = 1000

[bar]
format = "{hostname} {cwd} || {time}"   # left = before "||", right = after; no centre
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
- `bar.format` and `[[bar.block]]` are **mutually exclusive**: if blocks are
  present, `format` is rejected; otherwise `format` splits on `||` into a left and
  a right section (no centre block).
- `bar.height` must be `1` in the normal screen; alternate-screen behavior is fixed
  (bar hidden) and not configurable in the MVP.
- Module IDs are unique; a block references an enabled module by ID. Defaults are
  applied only for omitted values. An unavailable optional provider renders its
  fallback and emits a diagnostic — it never blocks terminal I/O.

## Structured block layout (spec §8.8)

This target-schema example uses post-MVP providers (git). `format` and `[[bar.block]]`
are mutually exclusive (spec §13.1) — use one or the other.

```toml
[bar]
height = 1

[[bar.block]]
module = "git"
anchor = "left"
width = "30%"
align = "left"
truncate = "right"
style = "git"

[[bar.block]]
module = "time"
anchor = "right"
width = 10
align = "right"
style = "time"
```

## Theme, icons, style (spec §8.9, §8.10; arch.md §16)

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

[theme.status]             # semantic tokens (arch.md §16)
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
bold = true
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
```

Custom commands run **locally** with a timeout; config is trusted user input but
commands must always be time-bounded (spec §16, §17).

`source = "time"` can reuse a built-in provider under a custom placeholder:

```toml
[module.time_utc]
source = "time"
format = "%H:%M UTC"
```

## Field map

The Go schema lives in `internal/config/schema.go` (`Config`, `BarConfig`,
`BlockConfig`, `ModuleConfig`, `ThemeConfig`, `IconsConfig`, `StyleConfig`).
Defaults in `default.go`.
