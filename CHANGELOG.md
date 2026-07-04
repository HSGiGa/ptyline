# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

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
