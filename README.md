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

## Features

- Transparent PTY wrapper: normal shell I/O, native scrollback, child exit code
  propagation, signal handling, and terminal restoration.
- Bottom status area with one or more rows, left/center/right sections,
  conditional separators, width/alignment suffixes, themes, styles, icons, and
  animations.
- Shell integration for bash, zsh, and fish through OSC 777 metadata.
- Built-in modules for cwd, env, command status, SSH state, git, time/date,
  identity/runtime labels, and system metrics.
- Custom local modules with bounded `source = "exec"` commands.
- Config overlays: command-line overlays and nearest project `.ptyline` files.
- Runtime reload with `ptyline --reload`.

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
ptyline --no-project-ptyline    # ignore nearest project .ptyline overlays
ptyline init fish               # print the fish shell-integration script
ptyline --reload                # reload config in the running ptyline
ptyline --version
```

Shell integration is optional; ptyline still works as a transparent wrapper
without it. Integration scripts emit whitelisted OSC 777 metadata for cwd,
environment, active/done command, exit code, duration, SSH state, and shell color
sync. Supported templates are `bash`, `zsh`, and `fish`.

Examples:

```sh
eval "$(ptyline init bash)"
eval "$(ptyline init zsh)"
ptyline init fish | source
```

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

Minimal config:

```toml
config_version = 1
shell = "auto"

[bar]
format = "{cwd} || {git} || {time}"
separator = " | "

[module.time]
format = "%H:%M:%S"
interval_ms = 1000

[module.cwd]
mode = "shell-integration"

[module.git]
format = "{branch}{dirty}"
```

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

Custom command module example:

```toml
[bar]
format = "{cwd} || {kube} || {time}"

[module.kube]
source = "exec"
command = "kubectl config current-context"
env = ["KUBECONFIG", "PATH"]
interval_ms = 10000
timeout_ms = 200
format = "{stdout}"
refresh_on_command = ["kubectl config use-context"]
```

Exec modules run locally from trusted config and are always time-bounded. By
default, a command inherits the environment of the `ptyline` process. If a tool
depends on environment that changes inside the interactive shell after startup
(for example `mise`, `direnv`, `aws-vault`, or project-local authentication),
declare the variables on that exec module:

```toml
[module.gh]
source = "exec"
command = "gh api user --jq .login"
env = ["GH_*", "GITHUB_*", "PATH", "XDG_CONFIG_HOME"]
refresh_on_command = ["gh auth login", "gh auth logout", "gh auth refresh"]

[module.aws]
source = "exec"
command = "aws sts get-caller-identity --query Account --output text"
env = ["AWS_*", "PATH"]
refresh_on_command = ["aws sso login", "aws-vault exec"]
```

The `env` list is a per-module allowlist, not a display setting. Each entry is
either an exact name (`GH_TOKEN`) or a prefix with a trailing `*` (`GH_*`); the
wildcard matches every currently-exported variable with that prefix. With shell
integration enabled, the shell reports the matching variables through the OSC 777
metadata channel on prompt updates and before/after commands, and the exec
command additionally runs from the shell's current directory. `ptyline` applies
the shell's latest snapshot when that exec module runs, so a later refresh sees
the active shell environment and cwd — this is what lets modules follow
directory-scoped tools such as `.mise.toml` or `direnv` without sending the
entire shell environment back to the wrapper. Values are base64-encoded and each
report carries a per-session nonce, so injected terminal bytes cannot forge them.
Because the report is a full snapshot, a variable that becomes unset (e.g. on
leaving a directory) is dropped on the next prompt.

Updates happen on the next normal module interval, and can happen immediately
after successful matching commands listed in `refresh_on_command`. For directory
changes that alter environment through prompt hooks, the new values are visible
after the shell integration emits its next prompt/preexec metadata. The list of
requested names is passed to the child shell when `ptyline` starts; restart the
wrapper after changing `env = [...]` entries.

Setting `refresh_on_cwd = true` re-runs an exec module the moment the shell's
directory changes. Because the command runs from that directory, this pairs well
with directory-scoped launchers that resolve the environment themselves — for
example `mise exec --`, which computes the directory's environment fresh at run
time (use it *instead of* `env = [...]` for that module, not alongside, so a
lagging mirrored value can't override what the launcher resolves):

```toml
[module.gh]
source = "exec"
command = "mise exec -- gh auth status --json hosts --jq '.hosts[][] | select(.active) | .login'"
refresh_on_cwd = true
```

### Multiline commands

`command` is passed verbatim to `/bin/sh -c`, so it can be a multiline shell
script. Use a TOML literal string (`'''…'''`) — it performs no escape processing,
so single quotes and backslashes (common in `jq`/`awk` snippets) pass through
untouched. TOML trims a newline immediately after the opening `'''`, so the
script can start on the next line:

```toml
[module.gh]
source = "exec"
command = '''
set -e
acct=$(mise exec -- gh auth status --json hosts --jq '.hosts[][] | select(.active) | .login')
printf '%s' "$acct"
'''
refresh_on_cwd = true
```

A trailing `\` continues a line at the shell level (TOML leaves it in place; `sh`
joins the lines). Basic multiline strings (`"""…"""`) also work but process
`\n`, `\t`, and `\"`, which is usually inconvenient for shell snippets.

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
