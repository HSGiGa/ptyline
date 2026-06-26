# 12 — Shell Integration Adapters
Status: [x] done
Depends on: 06, 07
Spec refs: spec §9, §14, §18, §24.1; docs/shell-integration.md

## Goal
`ptyline init <shell>` emits a working integration for bash, zsh, and fish that
reports cwd, exit code, command, and duration via OSC 777; the filter consumes the
whitelisted messages and the bar reflects accurate state. Integration stays
optional — ptyline still works with any shell/command without it.

## Deliverables
- `internal/shellintegration/templates/{bash,zsh,fish}.sh` — script templates
  served by `ptyline init <shell>`.
- **Data-driven registry**: replace the hand-maintained per-shell `//go:embed`
  vars + `map[string]string` in `osc.go` with `//go:embed templates/*.sh` into an
  `embed.FS`; `Script(shell)` reads `templates/<shell>.sh`; `Supported()` lists the
  embedded directory. Adding a shell becomes a template file with **zero Go edits**.
- (Filter OSC handling + whitelist already in plan 06 — but see Invariants: the
  whitelist/decode table is owned by `shellintegration` and consumed by the proxy,
  not redefined there.)

The templates are not Go adapters. The Go implementation is shell-agnostic: it
parses the common OSC protocol and updates `ShellState` for every supported shell.
See the **shell-agnostic contract** in `docs/shell-integration.md` — the four rules
there are the acceptance bar for this plan.

## Approach
1. **bash**: `PROMPT_COMMAND` prints `exit_code`, `cwd`; a `DEBUG` trap captures
   `command` + start time for `duration_ms`.
2. **zsh**: `precmd` (exit_code, cwd) + `preexec` (command, start → duration).
3. **fish**: `--on-event fish_postexec` (exit_code, command, duration) + PWD hook
   (cwd).
4. Frame with ST (`\e\\`); emit strict `key=value`; never echo executable content
   (spec §17). Keep keys within the whitelist.
5. **Normalize inside the template, not in Go.** Each template emits the canonical
   form — `exit_code` as a plain integer, `duration_ms` already computed, `cwd` as
   an absolute path — so Go consumes one representation and never special-cases a
   shell.

## Invariants
The wrapper never executes OSC values (spec §17). Works without integration too —
`cwd` shows its fallback until an adapter supplies metadata (spec §13).
**Shell-agnostic:** no Go path branches on the shell name; normalization lives in
the template; the registry is data-driven; the protocol whitelist/decode table has
a single owner (`shellintegration`), keyed by protocol key, never by shell.

## Acceptance
- [ ] `eval "$(ptyline init bash)"` (and zsh/fish equivalents) updates cwd, exit
  code, and duration on the bar.
- [x] Raw OSC sequences never appear on screen; non-whitelisted keys are ignored.
- [x] Adding a shell is a new `templates/<shell>.sh` only — no Go change makes
  `ptyline init <shell>` and `Supported()` pick it up (registry is data-driven).
- [x] No Go identifier or branch names a specific shell (grep stays clean).

## Tests
Snapshot the emitted scripts. **Canonical round-trip:** feed each shell's OSC output
through the one filter and assert it produces the **same** `ShellState` — a single
assertion set shared across all shells (proves normalization, not per-shell decode).
Assert rejection of bad keys / control chars. Add a throwaway `templates/_test.sh`
fixture and assert `Supported()`/`Script` surface it with no code change.

## Out of scope
Additional shell adapters (nushell, PowerShell) — post-MVP (spec §19).
