package runtimeenv

import "github.com/hsgiga/ptyline/internal/platform"

// Detect resolves the runtime Profile exactly once at startup. It delegates the
// OS-specific classification to the build-tagged platform package, then derives
// capability flags from the kind.
//
// TODO scaffold (plan 01): flesh out capability probing (terminal feature
// detection, procfs/sysfs availability, WSL interop).
func Detect() Profile {
	kind := classify(platform.Detect())
	return Profile{
		Kind:         kind,
		Capabilities: capabilitiesFor(kind),
	}
}

// classify maps the platform package's raw verdict to a normalized Kind.
func classify(p platform.Verdict) Kind {
	switch {
	case p.IsWindows:
		return NativeWindows
	case p.IsMacOS:
		return MacOS
	case p.IsLinux && p.IsWSL:
		return WSL2
	case p.IsLinux:
		return NativeLinux
	default:
		return Unknown
	}
}

// capabilitiesFor returns conservative default capability flags for a Kind.
func capabilitiesFor(k Kind) Capabilities {
	c := Capabilities{VTSequences: true}
	switch k {
	case NativeLinux, WSL2:
		c.UnixPTY = true
		c.LinuxProcfs = true
		c.LinuxSysfs = true
		c.WindowsInterop = k == WSL2
	case MacOS:
		c.UnixPTY = true
	case NativeWindows:
		c.WindowsConPTY = true
	}
	return c
}
