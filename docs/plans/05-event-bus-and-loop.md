# 05 — Event Bus & Loop
Status: [ ] not started
Depends on: 03, 04
Spec refs: spec §8.3, §7; arch.md §4; docs/event-bus.md

## Goal
A single event loop drives the program: it spawns producers (stdin, PTY, signals,
ticker) that feed the bus, forwards IO both ways, and exits with the child's code.
**Milestone: ptyline becomes a transparent PTY pass-through.**

## Deliverables
- `internal/proxy/eventloop.go` — full event dispatch.
- `internal/proxy/writer.go` — finish the **serialized terminal writer**
  (`WriteChild` retry loop, `FlushBarFrame` at safe boundaries, rate limit).
- Producer goroutines (in `proxy` or `app`): stdin→`StdinInput`, PTY→`PtyOutput`,
  `SIGWINCH`→`Resize` (debounced), `SIGTERM/SIGHUP`→`TerminationSignal`,
  ticker→`Tick`, supervisor `Wait`→`ChildExited`.
- `internal/app/app.go` — wire producers and run the loop (remove the
  "pipeline not yet wired" notice).

## Approach
1. Loop `select`s over `bus.Events()`.
2. `StdinInput`→write child PTY (Ctrl-C/Ctrl-Z ride through as bytes);
   `PtyOutput`→filter (plan 06)→`writer.WriteChild`→request redraw;
   `Resize`→`sup.Resize` + mode-specific scroll region; `Tick`→refresh + redraw;
   `TerminationSignal`→`sup.TerminateGroup` then return; `ChildExited`→return code.
3. All terminal output goes through the **one** `TerminalWriter`; never interleave a
   bar frame with a child-output write (spec §8.3). Coalesce/rate-limit redraws.
4. Debounce resize ~30–50ms (spec §12). Restore terminal + reap group on exit.

## Invariants
Only the loop mutates terminal/PTY state. A single serialized writer owns all
terminal writes; child bytes are never dropped/duplicated/reordered. Output
forwarding is never blocked by rendering. Restore + group reap run on all exit
paths (spec §8.3, §15).

## Acceptance
- [ ] `ptyline fish` gives a fully usable interactive shell (input/output, Ctrl-C).
- [ ] Resizing works; child sees `rows-1`.
- [ ] Exiting the shell restores the terminal and returns the child's code.

## Tests
Replay-style loop test (plan 14 harness) feeding scripted events; assert exit code
and that output is forwarded. Resize-debounce unit test.

## Out of scope
ANSI protection (plan 06) and the bar (plans 07–09). Until 06 lands, the reserved
row is NOT yet protected — keep this branch unmerged for daily use or land 06 next.
