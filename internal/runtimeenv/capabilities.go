package runtimeenv

// Capabilities are the feature flags components query instead of checking the OS
// name directly. Backends and modules ask "do I have unix_pty / linux_procfs?"
// rather than "am I on Linux / WSL?" (spec §4.2, arch.md §14).
type Capabilities struct {
	UnixPTY        bool
	WindowsConPTY  bool
	VTSequences    bool
	LinuxProcfs    bool
	LinuxSysfs     bool
	WindowsInterop bool

	// Terminal feature detection (arch.md §14). Populated later from termenv /
	// environment probing.
	OSC8Links       bool
	TrueColor       bool
	NerdFont        bool
	Emoji           bool
	Mouse           bool
	AlternateScreen bool
}
