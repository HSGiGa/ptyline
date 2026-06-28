// Package runtimeenv detects the runtime environment once at startup and exposes
// a normalized Profile plus Capabilities. The rest of the application depends on
// capabilities and selected backends, never on raw OS-name checks (spec §4.2,
// ARCHITECTURE.md §14).
package runtimeenv

// Kind is the normalized runtime classification.
type Kind int

const (
	Unknown Kind = iota
	NativeLinux
	WSL2
	MacOS
	NativeWindows
)

func (k Kind) String() string {
	switch k {
	case NativeLinux:
		return "native_linux"
	case WSL2:
		return "wsl2"
	case MacOS:
		return "macos"
	case NativeWindows:
		return "native_windows"
	default:
		return "unknown"
	}
}

// Profile is the resolved runtime description handed to backend selection.
type Profile struct {
	Kind         Kind
	Capabilities Capabilities
}
