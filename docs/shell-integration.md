# Shell Integration

Source: `internal/shellintegration` (+ `templates/`). Design: spec §9, §14, §24.1.

## Why & optionality

The wrapper sees only PTY bytes. It cannot reliably know the child's current
directory, the exit code of the last command, or how long a command ran. The
shell knows all of this, so an **adapter** reports it via OSC sequences the
ANSI/OSC filter consumes (spec §9).

Integration is **optional**: ptyline works with any shell or command (including
`ssh`, `vim`) without an adapter — the PTY wrapper itself requires none. Adapters
are independent and additive.

## Shell-agnostic PTY model

ptyline follows the same boundary as tmux: the PTY wrapper does not need to know
which shell it runs. It starts the configured command (or the user's login shell)
inside a PTY and proxies terminal I/O. The child may equally be `bash`, `zsh`,
`fish`, `nu`, `ssh`, `vim`, or any interactive program.

The status bar's base data comes from ptyline itself: terminal size, clock, hostname,
and module providers. Shell integration only adds data a shell can report accurately,
such as its current directory, the last exit code, and command duration. Absence of a
shell hook must never affect PTY startup, job control, or normal terminal I/O.

Consequently, there is no Go implementation per shell. The Go code owns one generic
OSC parser and `ShellState` updater. A shell-specific integration is only a small
script template that emits the common protocol.

## Shell-agnostic contract

"Shell-agnostic" is a binding invariant, not an aspiration. It holds iff all four
rules below hold; treat them as the definition of done for any shell-integration
change and as a checklist for adding a shell.

1. **The protocol is the only coupling.** No Go code path branches on the shell
   name (no `if shell == "zsh"`). The shell name exists in exactly two generic
   places — the template **filename** (`templates/<shell>.sh`) and the
   `ptyline init <shell>` argument — and nowhere in parsing, state, or rendering.

2. **Normalization happens in the template, never in Go.** Each template converts
   its shell's idioms into the **canonical** protocol representation before the
   bytes cross into Go: `exit_code` as a plain integer, `duration_ms` already
   computed (bash `EPOCHREALTIME`, zsh `$EPOCHREALTIME`, fish `date +%s%3N` are
   template concerns), `cwd` as an absolute path. Go consumes one canonical form
   and must never special-case a shell's quirks (e.g. signal-encoded exit codes).

3. **Adding a shell = adding a template file, with zero Go changes.** The template
   registry is therefore **data-driven**: `//go:embed templates/*.sh` into an
   `embed.FS`; `Script(shell)` reads `templates/<shell>.sh`; `Supported()` lists
   the embedded directory. There is no hand-maintained `map[string]string` and no
   per-shell `//go:embed` var to edit. Dropping `templates/nu.sh` makes
   `ptyline init nu` work on its own.

4. **One protocol authority.** The whitelist, framing, size/control-char rules, and
   the key→`ShellState` decode table live in `internal/shellintegration` only; the
   proxy filter consumes them rather than redefining its own copy. The decode table
   is keyed by **protocol key**, never by shell. Any future namespace (e.g. agent
   events, spec §24.5) is keyed by **event class**, never by shell.

These rules are enforced by tests, not just review: the round-trip test feeds every
shell's emitted OSC output through the one filter and asserts the **same** canonical
`ShellState` — a single assertion set covering all shells (see plan 12).

## Protocol (OSC 777)

```text
OSC 777 ; cwd=/home/u/project ST
OSC 777 ; exit_code=0 ST
OSC 777 ; duration_ms=153 ST
OSC 777 ; command=git status ST
```

- `ST` = `ESC \` (preferred) or `BEL`.
- **Whitelist:** only `cwd`, `exit_code`, `duration_ms`, `command` (constants in
  `internal/shellintegration/osc.go`).
- Values contain **no control characters**; payloads ≤ **8 KiB**.
- Unknown/malformed/oversized payloads are dropped with a diagnostic and **never**
  executed (spec §9, §17). Accepted messages update `StatusState.Shell` and are not
  forwarded to the terminal.

## Emitting the script

```sh
ptyline init bash    # or: zsh | fish — prints the script to stdout
```

Source it from the shell config, e.g.:

```sh
# bash:  eval "$(ptyline init bash)"
# zsh:   eval "$(ptyline init zsh)"
# fish:  ptyline init fish | source
```

## Shell support

Reference adapters: **bash**, **zsh**, **fish**. They are script templates at
`internal/shellintegration/templates/{bash,zsh,fish}.sh`, embedded as a directory
(`embed.FS`) and served by `ptyline init <shell>` purely by filename — the set of
supported shells is whatever templates exist, not a hardcoded list (see the
shell-agnostic contract above).
bash uses `PROMPT_COMMAND` + a `DEBUG` trap; zsh uses `precmd`/`preexec`; fish uses
`fish_postexec` / prompt events. Each emits the canonical protocol; the differences
are confined to the template. Additional adapters are post-MVP (spec §19) and are
added by dropping a template file.

## Graceful degradation

Without integration, cwd/exit/duration modules show their fallback (e.g. `cwd`
with `mode = "shell-integration"` renders empty) and never infer from arbitrary
PTY output (spec §13). Typed snapshots make "unavailable" explicit — see
[`state-model.md`](state-model.md). The bar still works; it just shows less.

## Future: agent events over the same channel (spec §24.5)

```text
OSC 777 ; agent.started={"id":"a1","name":"reviewer"} ST
OSC 777 ; agent.update={"id":"a1","status":"running","tokens":1200} ST
OSC 777 ; agent.done={"id":"a1","exit_code":0} ST
```

Also planned: a Unix-socket provider at `$XDG_RUNTIME_DIR/ptyline/events.sock` and
a command provider — see [`agents-future.md`](agents-future.md).
