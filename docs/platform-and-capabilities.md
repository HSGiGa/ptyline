# Platform & Capabilities

Source: `internal/platform`, `internal/runtimeenv`. Design: spec §4, §24.6.

## MVP scope: Linux/WSL only

The **MVP targets Linux and WSL/WSL2 only** (reference distro: Ubuntu 24.04).
Native **macOS and Windows/ConPTY are post-MVP** (spec §4, §19); their backends
must not delay or weaken the Linux/WSL MVP. The `darwin.go` / `windows.go` /
`spawn_windows.go` files exist as build-tagged stubs so the one codebase stays
coherent, but they are not part of MVP acceptance.

## One codebase, three binaries (post-MVP fan-out)

```text
GOOS=linux   → Linux binary   (Unix PTY backend + WSL2 runtime branch)   ← MVP
GOOS=darwin  → macOS binary   (Unix PTY backend)                         ← post-MVP
GOOS=windows → Windows binary (ConPTY backend, selected/tested separately) ← post-MVP
```

**WSL2 is not a separate build target.** It is a *runtime branch* inside the Linux
binary (spec §4.1). OS-specific detection lives only in `internal/platform`
(build-tagged `linux.go` / `wsl.go` / `darwin.go` / `windows.go`).

## Detect once, then depend on capabilities

At startup, `runtimeenv.Detect()` resolves a normalized profile:

```text
runtime detection → Profile{Kind} → Capabilities → backend selection
```

`Kind` ∈ `native_linux | wsl2 | macos | native_windows | unknown`.

Crucially, the rest of the app depends on **capability flags**, not OS names
(spec §4.2). Bad: `if wsl2 { ... }` scattered in modules. Good: a module asks
"do I have `linux_sysfs`?" or "which battery providers are available?".

## Capability flags (`runtimeenv.Capabilities`)

```text
backend:   unix_pty, windows_conpty, vt_sequences
linux:     linux_procfs, linux_sysfs
wsl:       windows_interop, host_windows_access
terminal:  osc8_links, truecolor, nerd_font, emoji, mouse, alternate_screen
```

Backend selection (PTY type, scroll-region behavior) keys off these, so adding a
platform or a terminal workaround is a matter of setting flags, not threading OS
checks through the code.

## WSL detection (`internal/platform/wsl.go`)

Checks `$WSL_DISTRO_NAME` and `/proc/sys/kernel/osrelease` for `microsoft`/`wsl`.
WSL-specific providers (e.g. battery via Windows interop) are gated behind
`windows_interop` / `host_windows_access`, not behind a raw "is WSL" check.

## Principle

Rely on **standard PTY and VT behavior**, not terminal-emulator-specific APIs
(spec §4). Capability detection exists to degrade gracefully (emoji/Nerd-Font/ASCII
fallback, no-color), not to special-case individual emulators.
