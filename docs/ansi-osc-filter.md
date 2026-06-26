# ANSI / OSC Filter

Source: `internal/proxy/ansi_filter.go`, `internal/proxy/osc.go`. Design: spec
§8.4, §9, §11, §16. This is the most safety-critical code — see
[`terminal-safety.md`](terminal-safety.md).

## Scope

A **lightweight** byte-stream filter on the child→terminal path. It is NOT a
terminal emulator. It only recognizes the sequences below and passes everything
else through untouched — including data that is not valid UTF-8 (spec §8.4).

## Sequences it must understand

| Sequence | Meaning | Action |
|---|---|---|
| `CSI r` | reset scroll region | **normal screen:** rewrite → `CSI 1 ; childRows r`; **alt screen:** pass through |
| `CSI t ; b r` | set scroll region | **normal screen:** clamp `b` to `≤ childRows`; **alt screen:** pass through |
| `CSI J` / `CSI K` | erase display / line | pass through |
| `CSI H` / `CSI f` | cursor position | pass through (bottom-bar mode needs no coord shift) |
| `ESC [?1049h/l` | enter/leave alt screen | toggle alt state → switch writer mode (below) |
| `ESC [?1047h/l`, `ESC [?47h/l` | legacy alt screen | same as above |
| OSC 777 ; … ST | shell integration | parse+whitelist, emit `ShellMeta`, **drop** from output |
| cursor save/restore | `ESC 7`/`ESC 8`, `CSI s`/`CSI u` | pass through |

`childRows = reserved.Area.ChildRows(rows)`. Example clamp (normal screen): child
sends `CSI 1 ; 30 r` on a 30-row terminal with 1 reserved row → emit `CSI 1 ; 29 r`.

## Normal vs alternate screen (spec §8.4, §11)

The MVP policy is **hide the bar in the alternate screen** and give the child the
full terminal. Therefore the filter's clamping is mode-dependent:

- **Normal screen** — clamp/rewrite scroll margins to protect the reserved row(s).
- **Alternate screen** — do **not** clamp; the child owns every row. The reserved
  area is not in effect.

On an alt-screen transition the filter signals the serialized writer, which runs
the entry/exit procedure (reset region + resize child to full rows on entry;
restore region `1..childRows` + resize to `childRows` + redraw on exit). See
[`terminal-safety.md`](terminal-safety.md) §3 and spec §11.

Fullscreen support is determined by these VT transitions, **not** by process name.
A program that draws fullscreen in the *normal* buffer (without the alt screen) is
best-effort only (spec §8.4).

## Partial sequences & limits

Reads do not align to escape-sequence boundaries. The filter buffers an incomplete
trailing sequence (`tail`) and prepends it to the next chunk; a sequence may span
several reads. Buffering is bounded — **`maxBufferedCSI` = 4 KiB** (spec §16);
oversized or malformed sequences are passed through unchanged after recording a
diagnostic (spec §15).

## OSC 777 protocol (spec §9)

```text
OSC 777 ; cwd=/home/u/project ST
OSC 777 ; exit_code=0 ST
OSC 777 ; duration_ms=153 ST
OSC 777 ; command=git status ST
OSC 777 ; env=APP_ENV=dev REGION=eu ST
```

- `ST` = `ESC \` (preferred) or `BEL`.
- **Whitelist:** only `cwd`, `exit_code`, `duration_ms`, `command`, `env` are accepted.
- Values must contain **no control characters**; payloads are capped at
  **8 KiB** (`maxOSCPayload`). Unknown/malformed/oversized messages are dropped
  with a diagnostic and **never** cause command execution (spec §9, §17).
- Accepted messages update `ShellState` and are removed from terminal output.

The same channel later carries agent events (`agent.started`/`agent.update`/
`agent.done`, spec §24.5).

## Testing

Prime target for the record/replay harness — feed recorded vim/less/htop/ssh
output and assert: reserved row never written (normal screen), no clamping in the
alt screen, split sequences reassemble identically regardless of read boundaries,
and OSC messages are consumed. See [`testing-and-replay.md`](testing-and-replay.md)
and the §20.1 verification matrix.
