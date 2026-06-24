# Glossary

Shared terms used across the docs, spec, and code.

- **Reserved area** — the row(s) at the terminal edge ptyline draws its bar on.
  Modeled by `reserved.Area{Edge, Rows}`; MVP is `{Bottom, 1}`. All PTY sizing is
  `terminalRows − reserved.Rows`.
- **Child PTY** — the pseudo-terminal the user's shell runs in. It is told it is
  `childRows = terminalRows − reserved.Rows` tall.
- **Scroll region (DECSTBM)** — `CSI top ; bottom r`; confines scrolling to a row
  range so output scrolls under the stationary bar.
- **ANSI/OSC filter** — the lightweight (non-emulator) byte filter on the
  child→terminal path that protects the reserved rows and consumes OSC messages.
- **Alternate screen** — the secondary screen buffer used by vim/less/htop/btop
  (`?1049h/l`). ptyline reapplies the scroll region on enter/leave.
- **OSC 777** — the shell-integration message format (`OSC 777 ; key=value ST`)
  carrying cwd/exit/duration/command (and future agent events).
- **StatusState** — the normalized, single read model the renderer consumes.
- **Module** — a provider of one bar value, refreshed on its own interval with a
  timeout; its result is a cached `ModuleSnapshot`.
- **ModuleValue** — a typed module result (`Text`/`Number`/`Bool`/`Status`), not a
  bare string.
- **Snapshot** — a cached module result with `UpdatedAt`/`Stale`/`Error` metadata.
- **Block** — a renderable unit on the bar with layout metadata (anchor, align,
  width, priority, style).
- **Anchor** — which side a block pins to (`left`/`center`/`right`).
- **Display width** — width measured in terminal cells (not bytes or runes); CJK
  and many emoji are 2 cells.
- **Theme token** — a semantic style name (`ok`, `warn`, `accent`, `agent.running`)
  the theme resolves into escape sequences.
- **Capability flag** — a runtime feature flag (`unix_pty`, `truecolor`,
  `nerd_font`, …) components query instead of checking the OS name.
- **Click zone** — a cell range on the bar mapped to an action (post-MVP mouse).
- **Reserved (Go `internal/reserved`)** — the package owning reserved-area math.
