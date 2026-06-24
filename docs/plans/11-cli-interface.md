# 11 — CLI Interface
Status: [ ] not started
Depends on: 05
Spec refs: spec §14

## Goal
The full CLI surface: run a shell/command, `--config`, `--version`, `--help`,
`init <shell>`, and `-- command` passthrough — with robust parsing.

## Deliverables
- `internal/app/cli.go` — harden `parseArgs` (support `--config=path`, clearer
  errors, `--help`/`-h`).
- `internal/app/app.go` — dispatch already in place; finalize messages.

## Approach
1. Behavior (spec §14): no args → configured shell or `$SHELL`; args → run as child
   inside the PTY; `init <shell>` → print integration script; exit with child code.
2. Keep parsing dependency-free (current hand-rolled parser) or adopt the stdlib
   `flag` package for flags before the child command.

## Invariants
Everything after `--` (or the first non-flag) is the child argv verbatim.

## Acceptance
- [ ] `ptyline`, `ptyline fish`, `ptyline -- bash`, `ptyline init fish`,
  `ptyline --version`, `ptyline --help` all behave per spec §14.
- [ ] Unknown flags / missing args produce a clear error and exit code 2.

## Tests
Extend `internal/app/cli_test.go` with `--config=path`, `--help`, error cases.

## Out of scope
`doctor` / `debug-state` / `replay` subcommands (plan 14 / post-MVP).
