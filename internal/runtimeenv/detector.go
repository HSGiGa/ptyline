package runtimeenv

import (
	"os"
	"strings"

	"github.com/hsgiga/ptyline/internal/platform"
)

// Detect resolves the runtime Profile exactly once at startup. It delegates the
// OS-specific classification to the build-tagged platform package, then derives
// capability flags from the kind.
func Detect() Profile {
	kind := classify(platform.Detect())
	capabilities := capabilitiesFor(kind)
	if kind == NativeLinux || kind == WSL2 {
		capabilities.LinuxProcfs = pathExists("/proc")
		capabilities.LinuxSysfs = pathExists("/sys")
	}
	capabilities.Color = detectColor(os.LookupEnv)
	capabilities.TrueColor = capabilities.Color == ColorTrue
	return Profile{
		Kind:         kind,
		Capabilities: capabilities,
	}
}

// detectColor classifies the terminal's color depth from the environment,
// honoring the NO_COLOR convention (https://no-color.org) and the de-facto
// COLORTERM/TERM signals. It takes a lookup func so it is testable without
// mutating the process environment.
func detectColor(lookup func(string) (string, bool)) ColorLevel {
	if v, ok := lookup("NO_COLOR"); ok && v != "" {
		return ColorNone
	}
	term, _ := lookup("TERM")
	if term == "dumb" {
		return ColorNone
	}
	switch colorterm, _ := lookup("COLORTERM"); strings.ToLower(colorterm) {
	case "truecolor", "24bit":
		return ColorTrue
	}
	if strings.Contains(term, "256color") {
		return Color256
	}
	if term != "" {
		return ColorBasic
	}
	return ColorNone
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
