# 04 — PTY Supervisor
Status: [x] done
Depends on: 00
Spec refs: spec §8.2, §6, §12; docs/terminal-safety.md

## Goal
Spawn the child shell in a PTY sized to `rows − reserved`, resize it on demand,
monitor its lifecycle, and report its exit code.

## Deliverables
- `internal/pty/spawn_unix.go` — real `creack/pty` start with initial winsize.
- `internal/pty/spawn_windows.go` — ConPTY start (can land later; stub for now).
- `internal/pty/{supervisor,resize}.go` — finished `Start`/`Resize`/`Wait`.

## Approach
1. Add `github.com/creack/pty`; `make tidy`.
2. `start(size)`: set `SysProcAttr{Setsid:true, Setctty:true}` so the child leads
   its own session/controlling-tty, then `pty.StartWithSize(cmd, &pty.Winsize{...})`,
   store ptmx.
3. `Resize(terminal)`: recompute `childSize` and `pty.Setsize`.
4. `Wait()`: `cmd.Wait()`, translate `*exec.ExitError` to an exit code.
5. `TerminateGroup`: on controlled shutdown, signal the child process group
   (`kill(-pgid, …)`) with a wait timeout, then reap.
6. Ensure `cmd` inherits the right env (`TERM`, etc.).

## Invariants
Child size is **always** `reserved.Area.ChildRows(rows)` in the normal screen
(never `rows-1` literal, never < 1) and full `rows` in the alt screen (spec §11).
The supervisor owns the child **process group** and preserves shell job control;
terminal-generated signals ride through as PTY bytes (spec §8.2). ptyline's exit
code equals the child's.

## Acceptance
- [ ] `ptyline fish` starts an interactive fish whose `$LINES` is `rows-1`.
- [ ] Resizing the terminal resizes the child to `rows-1`.
- [x] Exit code of the child propagates out of ptyline.

## Tests
Spawn `/bin/sh -c 'exit 7'` and assert `Wait()` returns 7. Assert `childSize`
math. PTY-backed test of initial winsize.

## Out of scope
Wiring reads/writes into the loop (plan 05). Full ConPTY parity (post-MVP for
Windows depth).
