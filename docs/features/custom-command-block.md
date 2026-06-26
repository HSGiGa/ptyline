# Custom Command Block

Status: proposed

## Goal

Allow users to add status blocks whose content comes from a local command.

Example use cases:

```text
kubectl config current-context
terraform workspace show
aws sts get-caller-identity --query Account --output text
python ~/bin/current-ticket.py
```

The command result becomes ordinary module content and can be placed in the bar
with the same layout and style rules as `{git}`, `{time}`, `{command}`, and other
blocks.

## User Model

Users define a named module and reference it from the bar:

```toml
[bar]
format = "{hostname} {cwd} {kube} || {time}"

[module.kube]
enabled = true
provider = "command"
command = "kubectl config current-context"
interval_ms = 10000
timeout_ms = 200
format = "{stdout}"

[style.kube]
fg = "base.fg"
bg = "accent"
padding_left = 1
padding_right = 1
```

Structured layout uses the same module ID:

```toml
[[bar.block]]
module = "kube"
anchor = "left"
width = "20"
truncate = "right"
style = "kube"
```

## Naming

Use `provider = "command"` on a normal `[module.<id>]`.

Do not require a special `[module.custom.<id>]` namespace in the final model. A
module ID such as `kube`, `aws`, or `ticket` is shorter in `{kube}` placeholders
and aligns with the existing module map.

The older documented shape:

```toml
[module.custom.kube]
```

may be kept as a compatibility alias if it already exists in user configs, but
the preferred form is:

```toml
[module.kube]
provider = "command"
```

## Execution Model

Custom commands are periodic providers:

- run on their own `interval_ms`;
- must have a positive `timeout_ms`;
- never run during rendering;
- publish cached `ModuleSnapshot` values;
- mark the snapshot stale or errored on timeout/failure;
- keep the previous successful value when a refresh fails, unless configured
  otherwise later.

Default values:

```text
interval_ms = 10000
timeout_ms  = 200
```

Commands run locally as trusted user configuration. OSC, socket input, child
terminal output, theme files, and project metadata must never trigger command
execution.

## Command Parsing

The `command` field is a shell command string executed through the user's command
shell or a documented fixed shell.

This is intentionally convenient for user config:

```toml
command = "git rev-parse --abbrev-ref HEAD 2>/dev/null"
```

If a future structured form is needed, it can be added without replacing the
string form:

```toml
argv = ["kubectl", "config", "current-context"]
```

For the first implementation, document exactly how the string is executed and
keep it consistent across platforms.

## Output Contract

The first implementation should treat stdout as plain text:

- trim trailing newlines;
- collapse internal newlines to spaces, or keep only the first line;
- reject control characters;
- cap captured stdout and stderr;
- render empty output as an empty block;
- never interpret ANSI escape sequences from command output.

Suggested limits:

```text
stdout_limit_bytes = 4096
stderr_limit_bytes = 4096
```

The default formatter exposes:

```text
{stdout}
{stderr}
{exit_code}
```

Typical config:

```toml
format = "{stdout}"
```

If the command exits non-zero, the module should set `Err` on the snapshot. The
renderer may hide the block, show the previous value as stale, or show a compact
error indicator depending on existing renderer policy.

## Security And Safety

Custom command execution is allowed only from trusted local config.

Required safeguards:

- positive timeout;
- output byte limits;
- no per-render execution;
- no command execution from OSC or socket events;
- no raw ANSI passthrough;
- diagnostics for timeout, non-zero exit, and output truncation.

Project-local config should not gain command execution automatically unless the
user explicitly opts into trusting that project. This prevents a cloned repository
from running commands just because ptyline starts inside it.

## Environment

Commands inherit a minimal, predictable environment:

- current working directory from ptyline's process, unless configured otherwise;
- standard environment variables from the parent process;
- no interactive stdin;
- stdout and stderr captured separately.

Future options may include:

```toml
cwd = "/path/to/project"
env = { KUBECONFIG = "/path/to/config" }
```

These are not required for the first implementation.

## Validation

Config validation should reject:

- `provider = "command"` without `command`;
- `command` with `timeout_ms <= 0`;
- `interval_ms <= 0`;
- command modules referenced by `bar.block` while disabled;
- unknown provider values.

Errors should include the module ID and offending key.

## Tests

Tests should cover:

- successful command refresh updates a cached snapshot;
- command timeout marks stale/error and does not block rendering;
- non-zero exit sets `Err`;
- stdout is trimmed and sanitized;
- oversized stdout/stderr are capped;
- renderer uses the cached value and does not execute commands;
- config validation requires a timeout for command providers.

## Non-Goals

- No remote command execution.
- No command execution from shell integration events.
- No streaming command output into the status bar.
- No interactive stdin.
- No raw ANSI passthrough from command output.
- No project-local command execution without an explicit trust decision.

## Relationship To Existing Docs

`docs/config-reference.md` and `ptyline-technical-spec.md` already reserve custom
command modules. This feature turns that reserved schema into a complete provider
contract for user-defined status blocks.
