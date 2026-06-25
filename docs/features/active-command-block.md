# Active Command Block

Status: implemented

## Goal

Add a status block that shows the command currently running in the foreground
inside the wrapped shell.

Examples:

```text
sleep 30
apt install nginx
npm run test
ssh user@host
```

The block is useful when the terminal is busy and the prompt is not visible: the
bar can still show what command is currently occupying the shell.

## Naming

Use `{cmd}` as the user-facing placeholder:

```toml
[bar]
format = "{hostname} {cwd} {cmd} || {git} {time}"
```

Use `active_command` as the internal module ID and config table name:

```toml
[module.active_command]
enabled = true
format = "{command}"
max_width = 40
animation = "glint"
animation_interval_ms = 80
```

Rationale:

- `{active}` is too generic;
- `{process}` implies low-level process inspection;
- `{foreground}` is accurate but long for a compact status format;
- `{cmd}` matches what users expect to see in a shell-oriented bar.

## User Model

The block is empty when the shell is idle. It becomes non-empty when a foreground
command starts, then clears when that command finishes.

Example timeline:

```text
prompt visible         -> {cmd} = ""
user runs "sleep 30"   -> {cmd} = "sleep 30"
command exits          -> {cmd} = ""
```

The command-duration and last-exit-code modules are separate features. This block
only represents the currently active command.

## Data Source

Primary source: shell integration.

The shell adapter should emit canonical OSC 777 events:

```text
OSC 777 ; command=sleep 30 ST
OSC 777 ; command= ST
```

The first event is emitted by pre-exec hooks when a command starts. The empty
event is emitted by pre-prompt/post-exec hooks when the command finishes.

The Go side must not parse arbitrary terminal output to guess commands. Without
shell integration, the module is unavailable/empty.

## State Model

`StatusState.Shell` should distinguish current and historical commands:

```text
Shell.ActiveCommand   string
Shell.LastCommand     string
Shell.LastExitCode    int
Shell.LastDurationMS  int
```

On command start:

- set `ActiveCommand`;
- optionally set `LastCommand` to the same value only if the existing state model
  needs a single compatibility field.

On command finish:

- move or keep the command in `LastCommand`;
- clear `ActiveCommand`;
- update `LastExitCode` and `LastDurationMS` from the existing shell events.

This avoids the common ambiguity where `LastCommand` is displayed as if it were
still running.

## Rendering

Default behavior:

- hidden when empty;
- truncated to the configured width;
- styled with a neutral or accent token;
- optionally animated with a left-to-right text glint while active;
- never includes control characters;
- never executes or re-parses the command string.

Suggested defaults:

```toml
[module.active_command]
enabled = true
format = "{command}"
max_width = 40
animation = "glint"
animation_interval_ms = 80

[style.active_command]
fg = "#f2b35d"
bg = "base.bg"
padding_left = 1
padding_right = 1
```

The display string should be sanitized before it reaches the renderer. OSC
payload validation already rejects control characters; the module should still
treat the value as plain text.

## Animation

The default animation is a text glint/gleam. A narrow lighter highlight sweeps
left-to-right across the active command text. The command string itself does not
change, move, or gain extra indicator characters; the display width remains
stable across animation frames.

The glint is activity-based. It starts when the command starts or writes output,
then stops after a short idle period while leaving the command text visible. This
keeps long-running interactive tools such as an idle agent prompt from looking
busy when they are waiting for input or doing no visible work.

Configuration:

```toml
[module.active_command]
animation = "glint" # glint | none
animation_interval_ms = 80
```

In no-color mode, glint degrades to static text. Setting `animation = "none"`
disables it.

The same animation fields are available on any module, for example:

```toml
[module.time]
animation = "glint"
animation_interval_ms = 120
```

For ordinary modules, glint runs continuously while the block is visible. For
`active_command`, glint is activity-gated: the command text remains visible while
the command is running, but the animation pauses after a short idle period.

## Shell Adapter Contract

Each shell template normalizes its native hooks into the same protocol:

- bash: DEBUG trap or equivalent pre-exec hook sets `command`;
- zsh: `preexec` sets `command`;
- fish: `fish_preexec` sets `command`;
- post-exec/pre-prompt clears `command` after exit and duration have been emitted.
- the clear event is the empty canonical payload `OSC 777 ; command= ST`.

The Go parser remains shell-agnostic: it consumes only the canonical `command`
key and must not special-case bash, zsh, fish, or any future shell.

## Edge Cases

- Multi-command lines may be shown as the original command line, for example
  `make test && git status`.
- Very long commands are capped by `module.active_command.max_width` and may be
  further truncated by layout; the shell adapter does not truncate them.
- Passwords or secrets typed directly into the command line can appear here, just
  like they can appear in shell history. A future config option may allow hiding
  commands matching configured patterns.
- Full-screen programs such as `vim` or `less` run in alternate screen; the bar is
  currently hidden there, so the block may not be visible while they are active.
- Nested shells report only what their own integration emits.

## Validation And Tests

Tests should cover:

- OSC `command=<value>` updates `Shell.ActiveCommand`;
- OSC `command=` clears `Shell.ActiveCommand`;
- control characters and oversized payloads are rejected by the existing OSC
  validation path;
- `{cmd}` renders empty while idle and renders/truncates while active;
- glint animation advances left-to-right without changing total display width;
- bash, zsh, and fish templates produce the same canonical protocol shape.

## Non-Goals

- No process-tree scanning for the MVP.
- No parsing of terminal output.
- No inference from prompt text.
- No command history UI.
- No special handling for package managers or long-running tools.

## Relationship To Existing Docs

`docs/shell-integration.md` already reserves the OSC 777 `command` key. This
feature turns that protocol field into a first-class status block and clarifies
that current command state should be separate from last command state.
