# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.9.5] - 2026-08-28

### Fixed

- The git module no longer takes `.git/index.lock` or rewrites `.git/index`
  while polling. `git status` refreshes the index on disk by default, so a
  bar tick could race a concurrent `git add`/`commit`/`rebase` in the same
  repository and make it fail with `Unable to create '.../index.lock': File
  exists`. Every git invocation now runs with `GIT_OPTIONAL_LOCKS=0`, which
  keeps the refresh in memory and leaves the repository untouched. Git calls
  also run with `GIT_TERMINAL_PROMPT=0` so they can never block the bar on a
  credential prompt.

## [0.9.4] - 2026-08-09

### Fixed

- A deep shrink resize (e.g. triggered by connecting/disconnecting a second
  monitor) could drop the input line onto ptyline's reserved bar rows, on
  terminals other than the previously known Terminal.app case (confirmed on
  iTerm2/WezTerm). A real terminal has to clamp the cursor when it no longer
  fits after a shrink, and it does so before ptyline reacts to the resulting
  resize, so blindly trusting cursor save/restore could preserve an
  already-clamped, already-wrong position. ptyline now asks the terminal
  where the cursor actually is after a shrink (a DSR query) and only
  preserves it when that's genuinely still inside the visible content area,
  falling back to the previous heuristic if the terminal doesn't answer in
  time.

## [0.9.3] - 2026-08-09

### Added

- `PTYLINE_DEBUG` now also traces cursor save/restore and resize events
  (scroll-region reapplication, bar repaints, the child's own untouched
  ESC7/ESC8, resize request/commit, and periodic module/tick redraws) to
  help diagnose a reported input-line-on-bar issue after a long
  minimize/sleep combined with a monitor reconfiguration. Diagnostic only —
  no behavior change when `PTYLINE_DEBUG` is unset.

## [0.9.2] - 2026-07-05

### Fixed

- The input line no longer drops to the bottom of the screen after a window
  resize on macOS. The resize path pinned the cursor to the last child row on
  every resize; now it preserves the cursor position (like Linux always did)
  and only pins on an actual shrink in Terminal.app, which clamps the cursor
  into the reserved bar row on shrink.

### Changed

- The pin-on-shrink workaround is now keyed on the hosting terminal
  (`$TERM_PROGRAM == "Apple_Terminal"`, exposed as
  `Capabilities.ClampsCursorOnShrink`) instead of a darwin build tag: iTerm2,
  WezTerm and other emulators on macOS respect the scroll region on shrink
  and keep the cursor in place, matching Linux behavior.

## [0.9.1] - 2026-07-04

### Fixed

- A module referenced only inside another module's template `format` (e.g.
  composing `cpu`/`memory`/`disk`/`load` into one custom block) is now
  auto-enabled the same way a top-level bar reference already was, and now
  keeps its own configured icon when rendered nested inside that template.
  Previously it silently stayed disabled and rendered as an empty, icon-less
  string.
- `enabled = true` no longer needs to be restated on `cpu`/`memory`/`disk`/
  `load`/`battery`/`command` when they're already referenced in the bar
  layout — `Load()` now infers it directly, matching the behavior project
  overlays already had.
- The command module's animation now defaults to an 80ms cadence instead of
  falling back to the generic 120ms, without needing `animation_interval_ms`
  stated explicitly.

### Changed

- Trimmed the shipped sample `config/config.toml` of values that only
  duplicated code-level defaults (`enabled`, `interval_ms`, `timeout_ms`,
  `max_width`, `done_*_ms`, `animation_interval_ms`).
- Replaced the `gh`-based exec-module example with a portable `uptime`
  command that works on both Linux and macOS without assuming a personal
  `mise`-managed `gh` install.

## [0.9.0] - 2026-07-03

Initial public release.
