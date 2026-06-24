package pty

// Platform-specific helpers (setsize, terminateGroup, exitCode) live in the
// build-tagged spawn_{unix,windows}.go files. The cross-platform Resize/Wait
// wrappers are defined in supervisor.go.
