# Terminal Safety — The Non-Negotiables

**Read this before editing `internal/terminal`, `internal/pty`, or `internal/proxy`.**
These rules come from spec §6, §8.1, §8.4, §8.6, §10, §11, §15. Breaking any of
them corrupts the user's live terminal — the worst class of bug this project can
ship.

## 1. The reserved-rows rule

The child PTY must believe it is shorter than the real terminal:

```text
childRows = terminalRows - reserved.Rows     (never < 1)
```

This is computed in exactly one place: `reserved.Area.ChildRows`
(`internal/reserved/area.go`). Never hardcode `rows - 1`. When the terminal is
smaller than the reserved area, clamp to 1 row rather than going negative
(spec §15: "terminal size smaller than 2 rows").

## 2. The scroll region (mode-dependent)

In the **normal screen**, set the real terminal scroll region to exclude the
reserved rows:

```text
CSI 1 ; childRows r        (terminal.SetScrollRegion(1, childRows))
```

In the **alternate screen** the bar is hidden and the child owns every row, so the
scroll region is **reset** (full screen). The correct region must be re-applied
every time the screen state changes:

- after a resize (mode-specific — spec §12),
- after the child enters/leaves the alternate screen (spec §11),
- whenever the child resets it (see the filter, below).

## 3. Alternate screen = hide the bar (MVP policy, spec §11)

The fixed MVP policy is to **hide the bar whenever the child enters the alternate
screen** and give the child full height. Determined by VT transitions, not process
name. The serialized writer runs:

**On entry:** stop scheduled redraws → reset scroll region → forward the enter
sequence → resize child PTY to `cols × rows` (full height).

**On exit:** forward the leave sequence → resize child PTY to `cols × max(rows-1,1)`
→ set scroll region to `1..childRows` (when ≥ 2 rows) → redraw the bar.

## 4. The ANSI/OSC filter protects the reserved rows (normal screen only)

The child can still try to draw on or scroll the reserved rows. The filter in
`internal/proxy` (see [`ansi-osc-filter.md`](ansi-osc-filter.md)) must:

- **normal screen:** rewrite a bare `CSI r` → `CSI 1 ; childRows r`, and clamp any
  `CSI t ; b r` whose `b` exceeds `childRows`;
- **alternate screen:** do NOT clamp — the child owns every row;
- track alternate-screen enter/leave (`?1049h/l`, `?1047h/l`, `?47h/l`) and signal
  the writer to run the entry/exit procedure above;
- intercept whitelisted OSC 777 messages (`cwd`/`exit_code`/`duration_ms`/`command`,
  ≤ 8 KiB, no control chars), update state, and **not** forward them;
- handle escape sequences split across read boundaries (buffer the incomplete tail,
  bounded by `maxBufferedCSI` = 4 KiB).

It is deliberately **not** a full VT emulator (spec §8.4).

## 4a. One serialized terminal writer (spec §8.3)

Every write to the real terminal — filtered child output **and** complete bar
frames — goes through a single `proxy.TerminalWriter`. This guarantees:

- a bar frame is never inserted into the middle of a child-output write;
- child bytes are never dropped, duplicated, or reordered (short/interrupted
  writes are retried);
- a redraw is emitted only at a safe event-loop boundary, rate-limited to ≤ 20 Hz,
  and skipped if the line is unchanged;
- bar frames are suppressed entirely while the alternate screen is active.

## 5. Drawing the bar

The renderer output is drawn with **absolute positioning and no trailing
newline** — a newline would scroll the bar into history (spec §8.6, §10.3):

```text
save cursor (ESC 7)
move to bar row, col 1 (CSI {barRow};1H)
clear line (CSI 2K)
write the rendered bar
reset attributes (CSI 0m)
restore cursor (ESC 8)
```

## 6. Always restore terminal state + reap the child group

Whatever ptyline changes, it restores — on normal exit, on `SIGINT`/`SIGTERM`/
`SIGHUP`, on child exit, and even if initialization fails *after* the terminal was
modified (spec §15). Required cleanup order (spec §8.1):

```text
reset scroll region (CSI r)
reset attributes (CSI 0m)
restore cursor (ESC 8)
restore terminal mode (term.Restore)
show cursor (CSI ?25h)
```

On controlled shutdown (wrapper `SIGTERM`/`SIGHUP`), terminate the child **process
group**, wait for it, then restore (spec §8.2). Restoration is best-effort for
uncatchable termination such as `SIGKILL`. `terminal.Controller.Restore` must be
**idempotent** (safe from a signal handler and again via `defer`).

Terminal-generated signals (Ctrl-C/Ctrl-Z) are forwarded to the PTY as input
bytes so the kernel delivers them to the child's foreground group — ptyline does
not synthesize them.

## 7. Exit code passthrough

ptyline exits with the child's exit code (spec §8.2). Signal-terminated runs use
the conventional `128 + signo` (e.g. 130 for SIGINT).

## Failure cases to handle (spec §15)

terminal < 2 rows · PTY creation failure · shell spawn failure · invalid config ·
module timeout · child exit · broken pipe (EPIPE) · interrupted syscalls (EINTR) ·
resize during render · partial CSI/OSC across reads · malformed/oversized ANSI
sequences · child process-group termination and wait timeout.
