# Event Bus & Event Loop

Source: `internal/event`, `internal/proxy/eventloop.go`. Design: ARCHITECTURE.md §4, spec §8.3.

## Why an event bus

Every input source is normalized into a single typed stream so the application
core never has to be rewritten to add a new source. The loop is the **only**
mutator of terminal/PTY/bar state, which removes hot-path locking and keeps the
renderer's view consistent.

## The event set (`event.AppEvent`)

A sealed interface — implementers live in `internal/event` so the loop can
exhaustively type-switch:

| Event | Producer | Loop reaction |
|---|---|---|
| `StdinInput{Data}` | stdin reader | write to child PTY |
| `PtyOutput{Data}` | PTY reader | `filter.Filter` → stdout → schedule redraw |
| `Resize{Cols,Rows}` | SIGWINCH (debounced) | resize PTY, reapply scroll region, redraw |
| `Tick{}` | status ticker | refresh modules, redraw |
| `ShellMeta{Key,Value}` | ANSI/OSC filter | update `StatusState.Shell` |
| `ModuleUpdated{ID,Snapshot}` | module refreshers | update `StatusState.Modules` |
| `ChildExited{Code}` | PTY supervisor `Wait` | return code, begin shutdown |
| `TerminationSignal{Signal}` | signal handler | return `128+signo`, shutdown |

Future sources reuse this mechanism without core changes (ARCHITECTURE.md §4): agent
updates, socket events, click actions, replay-driven events.

## The loop contract

- Runs in one goroutine; producers run in their own goroutines and only `Send`.
- Reads from `Bus.Events()` until `ChildExited`/`TerminationSignal`, then returns
  the exit code.
- Terminal restore is the caller's `defer` (in `app.run`) so it executes even on
  panic — the loop itself does not own cleanup.

## Backpressure (to decide in plan 05)

`PtyOutput` can be high-rate (e.g. `yes`, `cat bigfile`). Options: a large buffered
channel, coalescing redraw signals (at most one pending `Tick`/redraw), and
throttling bar redraws (spec §16). Output forwarding must never be blocked by bar
rendering — the bar redraw is throttled/skipped, output is not.

## Redraw throttling (spec §16)

- Coalesce redraws: many `PtyOutput`/`Tick` events collapse to one redraw per
  frame window.
- Skip the redraw entirely if the rendered line is byte-identical to the last one.
- Debounce `Resize` ~30–50ms before acting.
