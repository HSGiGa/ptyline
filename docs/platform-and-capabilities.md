# Platform & Capabilities

Source: `internal/platform`, `internal/runtimeenv`. Design: spec §4, §24.6.

## Supported scope: Linux, WSL, and macOS

The current readiness target is Linux, Linux/WSL, and macOS. WSL/WSL2 is a
runtime branch of the Linux binary. macOS uses the shared Unix PTY backend and
native system-metric providers backed by mach/IOKit.

Windows/ConPTY is deferred future work. The Windows files remain build-tagged
stubs so the platform boundary is explicit, but Windows is not part of current
readiness.

## One codebase, three platform paths

```text
GOOS=linux   → Linux binary  (Unix PTY backend + WSL2 runtime branch)
GOOS=darwin  → macOS binary  (Unix PTY backend + mach/IOKit metrics; cgo)
GOOS=windows → Windows binary (ConPTY backend) ← deferred, stubs
```

**WSL2 is not a separate build target.** It is a *runtime branch* inside the Linux
binary (spec §4.1). OS-specific detection lives only in `internal/platform`
(build-tagged `linux.go` / `wsl.go` / `darwin.go` / `windows.go`).

Binaries are built natively on each target platform. Linux is pure Go; macOS
requires `CGO_ENABLED=1` on a macOS host because the system-metric providers call
mach/IOKit.

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
