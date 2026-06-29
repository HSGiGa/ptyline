# ptyline

A lightweight terminal wrapper that reserves the bottom row(s) of your terminal
for a configurable status bar. It is intentionally smaller than tmux: no panes,
tabs, sessions, copy mode, or full terminal emulation. You keep your native shell
and native scrollback; ptyline pins a bar at the bottom.

```text
Terminal Emulator -> ptyline -> PTY -> fish / bash / zsh / vim / htop / ...
```

Status: active implementation. The PTY wrapper, status rendering, configuration,
shell integration, built-in modules, project overlays, reload path, and CI checks
are implemented for Linux, WSL2, and macOS. Windows/ConPTY, Agents, and
diagnostics/replay tooling are deferred.

## Quickstart

Requires Go 1.26+.

```sh
make bootstrap
make build
./dist/ptyline -- zsh
```

Common development commands:

```sh
make run ARGS='-- zsh'       # build and run with config/config.toml
make test                    # run unit tests
make check                   # format check, vet, tests, lint
make dist                    # build ptyline-<os>-<arch> for the current host
make test-one PKG=./internal/reserved RUN=TestChildRows
```

## Usage

```sh
ptyline                         # run the configured shell or $SHELL
ptyline fish                    # run fish inside the wrapper
ptyline -- bash                 # everything after -- is the child command
ptyline -- ssh host.example     # run any command inside the wrapper
ptyline --config ./config.toml  # use a specific config file
ptyline --ptyline compact       # apply a visual overlay
ptyline init fish               # print the fish shell-integration script
ptyline --reload                # reload config in the running ptyline
ptyline --version
```

Shell integration is optional; ptyline still works as a transparent wrapper
without it. Integration scripts emit whitelisted OSC 777 metadata for cwd,
environment, active/done command, exit code, duration, SSH state, and shell color
sync. Supported templates are `bash`, `zsh`, and `fish`.

## Configuration

Config is TOML. The default path is:

```text
$XDG_CONFIG_HOME/ptyline/config.toml
~/.config/ptyline/config.toml
```

The sample config lives at [config/config.toml](config/config.toml) and uses
[config/config.schema.json](config/config.schema.json) for editor validation.
The Go schema is defined in [internal/config/schema.go](internal/config/schema.go).

The effective config is layered:

```text
built-in defaults -> base config -> optional --ptyline overlay -> nearest project .ptyline
```

Project `.ptyline` files are visual/profile overlays. They may change bar rows,
module presentation, themes, icons, and styles, but they cannot choose child
commands or define command-executing modules.

## Bar Format

Bar rows use a compact placeholder format:

```toml
[bar]
separator = " | "

[[bar.row]]
separator = " : "
format = "{identity} || {env} | {runtime} | {shell} || {gh} | {time}"
```

Grammar:

```text
{module}  module placeholder
||        section split: left / center / right; not drawn
|         separator marker; draws the active row separator
\|        literal pipe character
```

Placeholders can include width/alignment suffixes such as `{cwd:<30}`,
`{git:^20}`, or `{time:>8}`. Empty neighboring modules collapse separator
markers, so the bar does not leave dangling separators.

## Modules

Built-in modules include:

- identity/runtime: `user`, `hostname`, `runtime`, `shell`, `cwd`, `env`;
- time: `time`, `date`, and custom `source = "time"` modules;
- shell state: `command`, `ssh`;
- VCS: `git` and git sub-placeholders;
- system metrics: `cpu`, `memory`, `disk`, `load`, `battery`;
- custom/local: `source = "exec"` and `source = "template"`.

Slow modules publish cached snapshots on their own interval. Rendering reads only
prepared state; it never shells out or probes the system directly.

## Platform Support

Supported today:

- Linux;
- WSL2, as a runtime branch of the Linux binary;
- macOS, via the shared Unix PTY backend and native mach/IOKit metric providers.

Linux builds are pure Go. macOS builds require a native macOS host with cgo
enabled. Windows/ConPTY is deferred; Windows files remain as build-tagged stubs
to keep the platform boundary explicit.

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - canonical package map and invariants.
- [config/config.schema.json](config/config.schema.json) - editor schema for the
  sample TOML config.
