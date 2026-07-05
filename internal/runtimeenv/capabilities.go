package runtimeenv

// ColorLevel is the terminal's color depth, detected from the environment. The
// theme degrades its palette to match (truecolor → 256 → 16 → none).
type ColorLevel int

const (
	ColorNone  ColorLevel = iota // NO_COLOR / dumb terminal: plain text only
	ColorBasic                   // 16 ANSI colors
	Color256                     // 256-color palette
	ColorTrue                    // 24-bit truecolor
)

func (c ColorLevel) String() string {
	switch c {
	case ColorBasic:
		return "16"
	case Color256:
		return "256"
	case ColorTrue:
		return "truecolor"
	default:
		return "none"
	}
}

// Capabilities are the feature flags components query instead of checking the OS
// name directly. Backends and modules ask "do I have unix_pty / linux_procfs?"
// rather than "am I on Linux / WSL?" (spec §4.2, ARCHITECTURE.md §14).
type Capabilities struct {
	UnixPTY        bool
	WindowsConPTY  bool
	VTSequences    bool
	LinuxProcfs    bool
	LinuxSysfs     bool
	WindowsInterop bool

	// Terminal feature detection (ARCHITECTURE.md §14). Color is probed from the
	// environment; NerdFont/Emoji cannot be detected reliably and are driven by
	// config (icons.preset), not set here.
	Color           ColorLevel
	OSC8Links       bool
	TrueColor       bool // convenience: Color == ColorTrue
	NerdFont        bool
	Emoji           bool
	Mouse           bool
	AlternateScreen bool

	// ClampsCursorOnShrink marks terminals that ignore the scroll region when the
	// window shrinks and clamp the cursor into the last physical row — which is
	// ptyline's reserved bar row. Known offender: Terminal.app (Apple_Terminal).
	// iTerm2, WezTerm, kitty and Linux terminals preserve the cursor, so the
	// resize path must not force it to the child bottom there. Detected from
	// $TERM_PROGRAM, not from the OS (spec §4.2: capabilities, not OS names).
	ClampsCursorOnShrink bool
}
