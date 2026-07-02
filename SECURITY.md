# Security Policy

## Supported Versions

ptyline is pre-1.0 / RC-stage. Security fixes land on `main` and in the latest
tagged RC or release. Older tags are not patched.

## Reporting a Vulnerability

Do not open a public issue for security reports.

Use GitHub private vulnerability reporting:
<https://github.com/hsgiga/ptyline/security/advisories/new>.
If that is unavailable, contact the maintainers privately and we will open an
advisory on your behalf.

Include:

- affected version or commit from `ptyline --version`;
- OS, terminal emulator, and shell;
- minimal config and child command;
- observed impact.

## Security Model

ptyline sits between the terminal emulator and the child PTY:

```text
Terminal Emulator -> ptyline -> PTY -> shell / child program
```

Configuration is trusted input. The base config can define `source = "exec"`
modules that run local shell commands on a timer. Only use configs you trust,
the same way you only source shell rc files you trust.

The child byte stream is untrusted parsed input. OSC 777 shell-integration
messages are whitelisted, size-bounded, and never executed. `exec_env` and `cwd`
frames carry a per-session nonce because they affect exec-module environments,
working directories, and project overlay lookup. Command output embedded in the
bar is sanitized before rendering.

Project `.ptyline` files are scope-restricted overlays. They cannot set command
execution fields such as `command`, `source`, `timeout_ms`, `refresh_on_command`,
`provider = "command"`, or `shell`. Use `--no-project-ptyline` to disable
project overlay discovery entirely.

## Scope

In scope:

- terminal-state corruption;
- escape-sequence injection through child output or OSC;
- command execution reachable from untrusted child output or project overlays;
- malformed-input denial of service.

Out of scope:

- arbitrary command execution requiring a malicious trusted base config;
- deferred Windows/ConPTY backend behavior.

