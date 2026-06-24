// Package proxy forwards bytes between the real terminal and the child PTY and
// runs the single event loop. Its most safety-critical piece is the ANSI/OSC
// filter, which protects the reserved row(s) and consumes shell-integration
// messages. It is intentionally NOT a full terminal emulator (spec §8.4).
package proxy

import "github.com/hsgiga/ptyline/internal/reserved"

// maxBufferedCSI caps a buffered, still-incomplete control sequence; oversized
// sequences are passed through after a diagnostic (spec §16: "maximum buffered
// CSI: 4 KiB", spec §15).
const maxBufferedCSI = 4 * 1024

// AltScreenState tracks whether the child is in the alternate screen. In the MVP
// the bar is HIDDEN while the alternate screen is active and the child owns every
// row, so the filter must NOT clamp scroll margins in that mode (spec §8.4, §11).
type AltScreenState struct {
	Active bool
}

// AnsiFilter inspects the child→terminal byte stream. Responsibilities (spec §8.4):
//
//   - in the NORMAL screen: rewrite a bare scroll-region reset (CSI r) to
//     CSI 1 ; bottom r, and clamp any region overlapping the reserved row(s);
//   - in the ALTERNATE screen: do NOT clamp — the child owns every row;
//   - track alternate-screen enter/leave (?1049h/l, ?1047h/l, ?47h/l) and signal
//     the writer to switch modes;
//   - intercept whitelisted OSC shell-integration messages and emit ShellMeta
//     instead of forwarding them to the real terminal;
//   - forward ordinary (even non-UTF-8) and unknown/malformed data unchanged,
//     after recording a diagnostic for the malformed case.
//
// It must handle partial sequences across read boundaries, so it buffers an
// incomplete tail between calls (bounded by maxBufferedCSI).
type AnsiFilter struct {
	area   reserved.Area
	rows   uint16 // current real-terminal rows
	alt    AltScreenState
	tail   []byte // buffered incomplete escape sequence
	onMeta func(key, value string)
}

// NewAnsiFilter creates a filter for the given reserved area and meta callback.
func NewAnsiFilter(area reserved.Area, onMeta func(key, value string)) *AnsiFilter {
	return &AnsiFilter{area: area, onMeta: onMeta}
}

// SetRows updates the known terminal height (after resize) so clamping targets
// the right bottom row.
func (f *AnsiFilter) SetRows(rows uint16) { f.rows = rows }

// AltActive reports whether the child is currently in the alternate screen.
func (f *AnsiFilter) AltActive() bool { return f.alt.Active }

// bottom is the last row the child is allowed to touch.
func (f *AnsiFilter) bottom() uint16 { return f.area.ChildRows(f.rows) }

// Filter processes child output and returns the bytes to write to the real
// terminal. Intercepted OSC messages are removed and reported via onMeta.
//
// TODO scaffold (plan 06): implement the incremental CSI/OSC parser with the
// `tail` carry-over (bounded by maxBufferedCSI); in the normal screen
// rewrite/clamp DECSTBM to `bottom()`, in the alternate screen leave margins
// untouched; toggle alt-screen; strip whitelisted OSC 777. Until then this is a
// pass-through (UNSAFE — does not yet protect the row).
func (f *AnsiFilter) Filter(in []byte) (out []byte) {
	return in
}
