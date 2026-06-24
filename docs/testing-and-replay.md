# Testing & Replay

Design: arch.md §18. Goal: deterministic tests of terminal behavior without a real
terminal or child process.

## Layers

1. **Pure unit tests** — the easy, high-value majority. Pure functions:
   `reserved.Area` math, CLI parsing, layout packing, display-width/truncation,
   theme-token resolution, config merge/migration, OSC parsing. No PTY needed.
2. **Filter tests** — drive the ANSI/OSC filter with byte streams and assert
   output bytes + emitted `ShellMeta`. Cover partial sequences across boundaries.
3. **Replay tests** — feed a recorded session through the event loop and assert
   terminal-level outcomes.

## Record/replay format (arch.md §18)

A recording is an ordered log of timed events:

```text
stdin input · PTY output · resize · tick · OSC event · child exit · signal
```

A replay test asserts properties such as:

- the reserved row is never written by child output;
- the scroll region is correct and reapplied after alt-screen/resize;
- the bar redraws when (and only when) content changes;
- OSC messages are consumed, not forwarded;
- terminal state is fully restored at the end.

## Programs worth recording

`vim`, `nvim`, `less`, `fzf`, `htop`, `btop`, `ssh` — they exercise alt-screen,
mouse, scroll regions, and dense escape sequences, and are the usual source of
corruption bugs.

## Determinism

The event loop consumes events from the bus; in tests the bus is fed from a
recording instead of live producers, and time/`Tick` are injected. No goroutine
timing, no real clock — same input, same output every run.

## Diagnostics in tests (arch.md §19)

Assert on `diagnostics.Record` (last module error, last render duration, ANSI
warnings) to catch silent degradations. These also back future `ptyline doctor` /
`ptyline replay <recording>` commands.
