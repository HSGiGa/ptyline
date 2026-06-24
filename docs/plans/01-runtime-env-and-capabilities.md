# 01 — Runtime Environment & Capabilities
Status: [x] done
Depends on: 00
Spec refs: spec §4, §4.1, §4.2; arch.md §14

## Goal
At startup the app resolves a normalized `runtimeenv.Profile` (Kind +
Capabilities) exactly once, and the rest of the code can branch on capabilities
instead of OS names.

## Deliverables
- `internal/platform/{linux,wsl,darwin,windows}.go` — real OS detection.
- `internal/runtimeenv/{detector,profile,capabilities}.go` — populate capability
  flags from Kind + probes.

## Approach
1. Implement `platform.Detect()` per OS (build tags). `wsl.isWSL()` checks
   `$WSL_DISTRO_NAME` and `/proc/sys/kernel/osrelease`.
2. In `runtimeenv.capabilitiesFor`, set backend flags (`unix_pty`,
   `windows_conpty`, `vt_sequences`) and probe `linux_procfs`/`linux_sysfs`,
   `windows_interop`.
3. Leave terminal-feature flags (`truecolor`, `nerd_font`, `emoji`, `mouse`,
   `osc8_links`) to plan 10 (termenv); default conservatively here.

## Invariants
Detection happens once; no scattered `if wsl` checks elsewhere (spec §4.2).

## Acceptance
- [x] On Linux, `Kind` is `native_linux` or `wsl2` correctly.
- [x] `unix_pty` true on linux/darwin; `windows_conpty` true on windows.
- [x] No package outside `platform`/`runtimeenv` references an OS name.

## Tests
Table tests for `classify` and `capabilitiesFor`; `isWSL` with a temp
osrelease file and env var.

## Out of scope
Terminal feature detection (plan 10). Battery/WSL-interop providers (post-MVP).
Native macOS/Windows classification beyond build-tagged stubs — those targets are
post-MVP (spec §4); the MVP exercises only `native_linux` / `wsl2`.
