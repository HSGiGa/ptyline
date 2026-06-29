# Security Policy

## Supported versions

ptyline is pre-1.0. Security fixes land on `main` and in the latest tagged
release. Older tags are not patched — please upgrade to the latest release.

## Reporting a vulnerability

**Please do not open a public issue for security reports.**

Use GitHub's private vulnerability reporting:
[**Report a vulnerability**](https://github.com/hsgiga/ptyline/security/advisories/new).
If that is unavailable, contact the maintainers privately and we will open an
advisory on your behalf.

Please include:

- affected version / commit (`ptyline --version`),
- platform (Linux distro or WSL2 version),
- a minimal reproduction (config snippet, child command, terminal),
- the impact you observed.

We aim to acknowledge a report within a few days and to keep you updated through
remediation and coordinated disclosure.

## Security model

ptyline sits between your terminal emulator and your shell:

```
Terminal Emulator → ptyline → PTY → shell / child program
```

The design treats two inputs very differently:

- **Configuration is trusted input.** Your base config
  (`$XDG_CONFIG_HOME/ptyline/config.toml`) can define `exec` modules that run
  local shell commands on a timer — by design (spec §17). Only run configs you
  trust, exactly as you would only `source` a shell rc file you trust.

- **The child's byte stream is untrusted parsed input.** Everything the child
  program writes (including OSC shell-integration messages) is parsed
  defensively. Notable boundaries:

  - **OSC 777 messages are parsed strictly and never executed.** Only a fixed
    whitelist of keys is accepted; values containing control characters or
    exceeding the size cap are dropped. A value is *never* interpreted as a
    command (`internal/proxy/osc.go`, `internal/app/exec_runtime.go`).
  - **Command output embedded in the bar is sanitized** — control characters,
    including `ESC`, are stripped so a child cannot inject escape sequences into
    the status line (`internal/modules/exec.go`).
  - The ANSI/OSC filter is intentionally **not** a full terminal emulator; it
    forwards unknown sequences unchanged and bounds buffered/oversized sequences.

- **Project-local config is scope-restricted.** A `.ptyline` file discovered in
  the current directory tree is loaded only as an **overlay**, and overlays are
  forbidden from setting command-execution fields (`command`, `source`,
  `timeout_ms`, `refresh_on_command`, `provider = "command"`, `shell`) — see
  `ValidateOverlayScope` in `internal/config/loader.go`. This means **`cd`-ing
  into an untrusted repository cannot make ptyline execute new commands.** Only
  your own trusted base config can introduce command execution.

  If you still prefer to disable project-local config entirely, run with
  `--no-project-ptyline`.

## Scope

In scope: terminal-state corruption, escape-sequence injection via child output
or OSC, command execution reachable from untrusted input (child stream or
project overlays), and denial of service triggered by malformed input.

Out of scope: arbitrary command execution that requires a malicious **base**
config (that is the trusted-config model above), and the not-yet-functional
macOS/Windows backends (post-MVP stubs, spec §19).
