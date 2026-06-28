# Command Block

Status: implemented

## Goal

Add a state-aware status block that shows the foreground command while it runs,
then shows the last completed command with exit status and duration.

Use `{command}` as the user-facing placeholder:

```toml
[bar]
format = "{hostname} {cwd} {command} || {git} {time}"

[module.command]
enabled = true
format = "{active} {last} | {exit} | {duration}"
separator = "•"
max_width = 60
done_min_duration_ms = 2000
done_success_ttl_ms = 3000
done_failure_ttl_ms = 0
animation = true
animation_interval_ms = 80

[style.command]
animation = "glint"
```

## User Model

`{command}` is empty before any command has run. It becomes active when a
foreground command starts, then switches to completed output when that command
finishes.

Example timeline:

```text
prompt visible             -> {command} = ""
user runs "npm test"       -> {command} = "npm test"
command exits with code 2  -> {command} = "npm test exit 2 8.4s"
```

The same format is used for all states:

```toml
format = "{active} {last} {exit} {duration}"
```

Use `|` as a conditional separator marker in command formats:

```toml
format = "{active} {last} | {exit} | {duration}"
separator = "•"
```

Empty fields drop adjacent separator markers, so an active `sleep 100` renders as
`sleep 100`, while a completed interrupted command can render as
`sleep 100 • sigint • 1s`.

State controls which placeholders are populated:

- active: `{active}` only;
- done: `{last}`, `{exit}`, and `{duration}`;
- idle/no-history: all fields empty.

After expansion, redundant whitespace is collapsed so one format works across
states.

## Done Visibility

Completed command output is intentionally temporary for successful commands:

```toml
[module.command]
done_min_duration_ms = 2000
done_success_ttl_ms = 3000
done_failure_ttl_ms = 0
```

Rules:

- successful commands shorter than `done_min_duration_ms` are hidden;
- successful commands at or above the threshold are shown for
  `done_success_ttl_ms`;
- failed commands are shown until the next command when `done_failure_ttl_ms = 0`;
- setting `done_failure_ttl_ms` to a positive value makes failed commands expire
  too.

This follows common prompt behavior: routine successful commands do not linger in
the status bar, while failures stay visible.

## Data Source

Primary source: shell integration.

The shell adapter emits canonical OSC 777 events:

```text
OSC 777 ; command=npm test ST
OSC 777 ; duration_ms=8420 ST
OSC 777 ; exit_code=2 ST
OSC 777 ; command= ST
```

The Go side must not parse arbitrary terminal output to guess commands. Without
shell integration, the module is unavailable/empty.

## State Model

`StatusState.Shell` stores the command lifecycle:

```text
Shell.ActiveCommand
Shell.LastCommand
Shell.LastExitCode
Shell.LastDurationMS
```

Rendering rules:

- `ActiveCommand != ""`: show active command and suppress completed fields;
- `ActiveCommand == "" && LastCommand != ""`: show completed fields;
- `ActiveCommand == "" && LastCommand == ""`: hide the block.

## Animation

`module.command.animation = true` animates while the command is active and doing
work. A newly-started command gets a short grace window, so silent commands like
`sleep 2` still look active. After that, an active command that stops producing
output becomes idle: the command text remains visible, but animation is
suppressed. Completed command output is static. The default command effect is
`glint`; set `[style.command].animation` to `pulse`, `blink`, or `glint` to
choose the visual effect.

In no-color mode, animation degrades to static text. Setting `animation = false`
or `animation = "none"` disables it.

## Future Rich Text

Per-placeholder styling is reserved for the rich text/span renderer:

```toml
[module.command.active]
animation = true
fg = "#f2b35d"

[module.command.exit]
ok_fg = "ok"
error_fg = "error"
```

See [`rich-text-span-renderer.md`](rich-text-span-renderer.md).

## Non-Goals

- No process-tree scanning.
- No parsing of terminal output.
- No inference from prompt text.
- No command history UI.
- No special handling for package managers or long-running tools.
