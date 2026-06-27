# Project Bar Overlays

Status: planned

## Goal

Allow users to switch bar layouts with `.ptyline` overlay files and allow
projects to provide a nearest `.ptyline` overlay that ptyline can apply while it
is running.

The main config remains the trusted full configuration. `.ptyline` files are
visual/profile overlays: they may change bar presentation and module display
settings, but custom command modules must be declared in the main config.

## User Model

The main config is selected with the existing CLI flag:

```sh
ptyline --config ~/.config/ptyline/config.toml
```

A visual overlay can be selected separately:

```sh
ptyline --ptyline ~/.config/ptyline/compact.ptyline
ptyline --ptyline ~/.config/ptyline/full.ptyline
```

Short names may resolve relative to the ptyline config directory:

```sh
ptyline --ptyline compact
```

which resolves as:

```text
$XDG_CONFIG_HOME/ptyline/compact.ptyline
~/.config/ptyline/compact.ptyline
```

When shell integration reports a `cwd` change, ptyline also searches that
directory and its parents for the nearest project `.ptyline`. If found, that
project overlay is applied on top of the main config and the selected CLI
overlay.

## Overlay Files

All `.ptyline` overlay files use the same patch format, whether selected by CLI
or discovered in a project.

Compact example:

```toml
config_version = 1

[bar]
format = "{cwd} || {git} || {time}"

[module.env]
env = ["APP_ENV"]
```

Full multi-line example:

```toml
config_version = 1

[[bar.row]]
format = " {command} || || {git} "
fill = "-"

[[bar.row]]
format = "{ssh} || {user}@{hostname} {cwd} || {env} {runtime} {shell} || {time}"
```

Project example:

```toml
config_version = 1

[bar]
format = "{cwd} || {git} || {env} {time}"

[module.env]
env = ["APP_ENV", "REGION"]
```

## Layering

Configuration should be layered as:

1. built-in defaults;
2. main config from `--config` or the default config path;
3. optional CLI overlay from `--ptyline`;
4. nearest project `.ptyline`, if discovered from the current `cwd`;
5. infer active modules from the resolved bar layout;
6. validate the resolved config;
7. redraw the bar.

The nearest project `.ptyline` has the highest precedence. This lets a user run
with a global preset such as `compact.ptyline` while allowing a specific project
to adjust its displayed environment variables or bar layout.

Add a CLI escape hatch to disable project discovery:

```sh
ptyline --no-project-ptyline
```

## Live Reload

ptyline should be able to re-resolve and redraw the bar while it is running.

Initial reload triggers:

- on shell-integration `cwd` events, discover the nearest project `.ptyline`;
- if the project overlay path changes, rebuild the resolved config and redraw;
- reread the selected `--ptyline` overlay and the project `.ptyline` during this
  rebuild, so edits can be picked up by changing directory out and back.

File watching is not required for the first version. A later version can add
watchers or a manual reload signal if needed.

## Overlay Scope

`.ptyline` overlays may override presentation and module-display settings:

- `[bar]` layout, including `format`, `[[bar.row]]`, fill, separator, padding,
  and structured blocks;
- `[theme]`, `[icons]`, and `[style.<id>]` visual settings;
- module display settings such as `enabled`, `format`, `mode`, `env`,
  `interval_ms`, `max_width`, `animation`, and `animation_interval_ms`.

`.ptyline` overlays must not override process-level or command-execution
settings:

- `shell`;
- `refresh_interval_ms`;
- `module.*.command`;
- `module.*.provider = "command"`;
- command-only execution settings such as `timeout_ms`.

This split keeps overlay files useful for visual context while avoiding hidden
command execution from files that may be committed to a repository.

## Custom Commands

Custom command modules are allowed only in the main config:

```toml
[module.kube]
command = "kubectl config current-context"
interval_ms = 10000
timeout_ms = 200
```

An overlay may reference or restyle a command module that already exists in the
main config:

```toml
config_version = 1

[bar]
format = "{cwd} || {kube} || {time}"

[module.kube]
format = "{value}"
max_width = 32
```

It may not create that command module by setting `command` locally.

## Module Activation

`enabled = true` should not be required for modules that appear in the resolved
bar layout. A module is active when it is referenced by:

- `bar.format`;
- one of the configured bar rows;
- a structured block.

`enabled = false` remains useful as an explicit overlay override:

```toml
config_version = 1

[module.git]
enabled = false
```

The loader should infer active modules after applying all overlays and then
validate that every referenced module is known or constructible from built-in
defaults.

## Merge Rules

Map-like sections merge by key:

- overlay `[module.<id>]` patches the base module;
- overlay `[style.<id>]` patches the base style;
- overlay theme and icon tables patch the base visual config.

Slice-like fields replace as whole values:

- `env = [...]` replaces the module env list;
- `[[bar.row]]` replaces the full row list.

No index-level row merge or append syntax is planned initially.

## Compatibility

Existing full config files continue to use `[bar]` directly.

Existing `.ptyline` files that only set `[bar].format` remain valid overlays.
Existing `enabled = true` entries should continue to parse, but examples should
prefer the inferred activation model.

## Non-Goals

- No named bar registry in the first version.
- No trust mechanism in the first version.
- No project-local custom command definitions.
- No project-local child shell selection.
- No automatic command execution from repository-owned `.ptyline` files.
