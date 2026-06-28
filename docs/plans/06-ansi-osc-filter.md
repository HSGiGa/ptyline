# 06 — ANSI / OSC Filter
Status: [x] done
Depends on: 05
Spec refs: spec §8.4, §9, §11; ARCHITECTURE.md §11.1; docs/ansi-osc-filter.md, docs/terminal-safety.md

## Goal
The child→terminal stream is filtered so the reserved row(s) cannot be corrupted,
alt-screen transitions are tracked, and OSC 777 shell-integration messages are
consumed (not forwarded). **This makes the pass-through safe.**

## Deliverables
- `internal/proxy/ansi_filter.go` — incremental CSI/OSC parser with `tail`
  carry-over; scroll-region rewrite/clamp; alt-screen tracking.
- `internal/proxy/osc.go` — OSC 777 framing + `parseOSC777` wired to `onMeta`.

## Approach
1. Implement a small state machine over bytes: ground → ESC → CSI/OSC.
2. CSI `r` **in the normal screen**: if no params → rewrite to `1 ; childRows`; if
   `t;b` with `b>childRows` → clamp `b`. **In the alt screen: pass through** (child
   owns every row).
3. Alt-screen `?1049h/l` (and `?1047`, `?47`): toggle state; signal the writer to
   run the entry/exit procedure (spec §11; plan 13).
4. OSC `777;key=value` ST: whitelist `cwd/exit_code/duration_ms/command`, reject
   control chars and payloads > 8 KiB, call `onMeta`, drop from output.
5. Buffer incomplete sequences in `tail`, bounded by `maxBufferedCSI` (4 KiB);
   oversized/malformed → pass through after a diagnostic (spec §15, §16).
6. Pass everything else through unchanged (including non-UTF-8).

## Invariants
**Read docs/terminal-safety.md.** Never forward whitelisted OSC 777. In the normal
screen, never let a scroll region include the reserved rows; in the alt screen,
never clamp. Handle sequences split across reads. Not a full emulator.

## Acceptance
- [x] `vim`, `less`, `fzf`, `htop` (alt screen) run without corrupting the terminal
  and get full height (spec §20.7).
- [x] Normal-screen child `CSI 1;30 r` on a 30-row terminal becomes `CSI 1;29 r`;
  the same sequence in the alt screen passes through unchanged.
- [x] Whitelisted OSC 777 messages update state and never appear on screen;
  non-whitelisted/oversized ones are dropped with a diagnostic.

## Tests
Filter unit tests: scroll-region rewrite/clamp; alt-screen toggle; OSC parse;
partial-sequence reassembly. `vim`/`htop` alt-screen behavior was manually
verified; recorded vim/less/htop replay fixtures remain in plan 14.

## Out of scope
Coordinate shifting (not needed for bottom-bar mode). Agent OSC events (plan 16).
