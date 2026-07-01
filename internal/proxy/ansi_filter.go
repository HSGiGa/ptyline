// Package proxy forwards bytes between the real terminal and the child PTY and
// runs the single event loop. Its most safety-critical piece is the ANSI/OSC
// filter, which protects the reserved row(s) and consumes shell-integration
// messages. It is intentionally NOT a full terminal emulator (spec §8.4).
package proxy

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/reserved"
)

// maxBufferedCSI caps a buffered, still-incomplete control sequence; oversized
// sequences are passed through after a diagnostic (spec §16: "maximum buffered
// CSI: 4 KiB", spec §15).
const maxBufferedCSI = 4 * 1024

const (
	escByte = 0x1b
	belByte = 0x07
)

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
	area         reserved.Area
	rows         uint16 // current real-terminal rows
	alt          AltScreenState
	tail         []byte // buffered incomplete escape sequence
	deferred     []byte // bytes after an alt transition, replayed after the transition is applied
	meta         []event.ShellMeta
	barClobbered bool // child emitted an erase that may have wiped the reserved bar rows
	onAlt        func(active bool)
	onDiag       func(msg string)
}

// NewAnsiFilter creates a filter for the given reserved area.
func NewAnsiFilter(area reserved.Area) *AnsiFilter {
	return &AnsiFilter{area: area}
}

// SetAltHandler registers the callback invoked when the child enters/leaves the
// alternate screen (the writer/loop runs the entry/exit procedure — spec §11).
func (f *AnsiFilter) SetAltHandler(fn func(active bool)) { f.onAlt = fn }

// SetDiagHandler registers a sink for malformed/oversized-sequence diagnostics.
func (f *AnsiFilter) SetDiagHandler(fn func(msg string)) { f.onDiag = fn }

// SetRows updates the known terminal height (after resize) so clamping targets
// the right bottom row.
func (f *AnsiFilter) SetRows(rows uint16) { f.rows = rows }

// SetArea updates the reserved area (after a bar-height change).
func (f *AnsiFilter) SetArea(area reserved.Area) { f.area = area }

// AltActive reports whether the child is currently in the alternate screen.
func (f *AnsiFilter) AltActive() bool { return f.alt.Active }

// TakeBarClobbered reports whether child output since the last call emitted an
// erase that ignores the scroll region and may have wiped the reserved bar rows
// (a cursor-to-end CSI 0 J), then resets the flag. The event loop uses it to force
// an immediate bar repaint so the bar does not stay blank until the next content
// change (spec §8.4; e.g. fish redrawing a multiline command on history steps).
func (f *AnsiFilter) TakeBarClobbered() bool {
	if !f.barClobbered {
		return false
	}
	f.barClobbered = false
	return true
}

// HasDeferred reports whether Filter stopped at an alt-screen transition and
// kept later bytes for a second pass. The event loop uses this to apply the
// transition before writing the rest of the PTY chunk.
func (f *AnsiFilter) HasDeferred() bool { return len(f.deferred) > 0 }

// DrainMeta returns shell metadata consumed during Filter calls since the last
// drain. The event loop applies these directly instead of sending them back
// through its own bus, so metadata cannot be dropped under bus backpressure.
func (f *AnsiFilter) DrainMeta() []event.ShellMeta {
	if len(f.meta) == 0 {
		return nil
	}
	meta := f.meta
	f.meta = nil
	return meta
}

// bottom is the last row the child is allowed to touch.
func (f *AnsiFilter) bottom() uint16 { return f.area.ChildRows(f.rows) }

func (f *AnsiFilter) diag(msg string) {
	if f.onDiag != nil {
		f.onDiag(msg)
	}
}

// Filter processes child output and returns the bytes to write to the real
// terminal. Intercepted OSC 777 messages are removed and reported via onMeta;
// scroll-region sequences are rewritten/clamped in the normal screen; alt-screen
// transitions are tracked. Sequences split across reads are buffered in `tail`.
func (f *AnsiFilter) Filter(in []byte) []byte {
	var data []byte
	if len(f.deferred) > 0 {
		data = append(f.deferred, in...)
		f.deferred = nil
	} else if len(f.tail) > 0 {
		data = append(f.tail, in...)
		f.tail = nil
	} else {
		data = in
	}

	out := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		b := data[i]
		if b != escByte {
			out = append(out, b)
			i++
			continue
		}

		// An escape sequence starts here. Try to delimit a complete one.
		n, complete := scanEscape(data[i:])
		if !complete {
			rest := data[i:]
			if len(rest) > maxBufferedSeq(rest) {
				// Oversized/malformed: stop buffering and pass through (spec §15).
				f.diag(fmt.Sprintf("oversized escape sequence (%d bytes) passed through", len(rest)))
				out = append(out, rest...)
				break
			}
			// Buffer the incomplete tail for the next read.
			f.tail = append(f.tail[:0], rest...)
			break
		}

		seq := data[i : i+n]
		var altChanged bool
		out, altChanged = f.handleSequence(seq, out)
		i += n
		if altChanged && i < len(data) {
			f.deferred = append(f.deferred[:0], data[i:]...)
			break
		}
	}
	return out
}

// maxBufferedSeq returns how many bytes of an incomplete escape sequence may be
// buffered before it is treated as oversized. OSC/DCS-style string sequences can
// legitimately carry up to maxOSCPayload (shell-integration metadata), so a 4–8
// KiB OSC 777 must be buffered until complete and consumed rather than leaking
// to the real terminal; CSI and simple escapes use the smaller maxBufferedCSI.
func maxBufferedSeq(rest []byte) int {
	if len(rest) >= 2 {
		switch rest[1] {
		case ']', 'P', 'X', '^', '_':
			return maxOSCPayload
		}
	}
	return maxBufferedCSI
}

// scanEscape, given a slice whose first byte is ESC, returns the length of a
// complete escape sequence and whether it is complete. CSI and OSC/string
// sequences may be incomplete (need more bytes); simple two/three-byte escapes
// resolve immediately.
func scanEscape(b []byte) (n int, complete bool) {
	if len(b) < 2 {
		return 0, false
	}
	switch b[1] {
	case '[': // CSI: params/intermediates (0x20-0x3f) then final (0x40-0x7e)
		for j := 2; j < len(b); j++ {
			if b[j] >= 0x40 && b[j] <= 0x7e {
				return j + 1, true
			}
		}
		return 0, false
	case ']': // OSC: terminated by ST (ESC \) or BEL
		return scanString(b)
	case 'P', 'X', '^', '_': // DCS/SOS/PM/APC: also ST-terminated strings
		return scanString(b)
	default:
		// Simple escape: optional intermediates (0x20-0x2f) then a final byte.
		j := 1
		for j < len(b) && b[j] >= 0x20 && b[j] <= 0x2f {
			j++
		}
		if j >= len(b) {
			return 0, false
		}
		return j + 1, true
	}
}

// scanString delimits an OSC/DCS-style string sequence terminated by BEL or
// ST (ESC \).
func scanString(b []byte) (n int, complete bool) {
	for j := 2; j < len(b); j++ {
		switch b[j] {
		case belByte:
			return j + 1, true
		case escByte:
			if j+1 < len(b) {
				if b[j+1] == '\\' {
					return j + 2, true
				}
				// Some other ESC — treat as terminator boundary for safety.
				return j, true
			}
			return 0, false // ESC at the very end: need the next byte
		}
	}
	return 0, false
}

// handleSequence dispatches one complete escape sequence, appending the bytes to
// forward (possibly rewritten, possibly nothing) to out. altChanged reports that
// the sequence entered or left the alternate screen.
func (f *AnsiFilter) handleSequence(seq, out []byte) ([]byte, bool) {
	if len(seq) < 2 {
		return append(out, seq...), false
	}
	switch seq[1] {
	case '[':
		forward, altChanged := f.handleCSI(seq)
		return append(out, forward...), altChanged
	case ']':
		return f.handleOSC(seq, out), false
	default:
		return append(out, seq...), false
	}
}

// handleCSI rewrites/clamps DECSTBM and tracks alt-screen toggles. It returns the
// bytes to forward (unchanged for everything it does not touch).
func (f *AnsiFilter) handleCSI(seq []byte) ([]byte, bool) {
	final := seq[len(seq)-1]
	params := string(seq[2 : len(seq)-1]) // between "ESC[" and the final byte

	switch final {
	case 'r': // DECSTBM (set scroll region)
		if f.alt.Active || strings.HasPrefix(params, "?") {
			return seq, false // alt screen: child owns every row; private 'r' is unrelated
		}
		return f.rewriteScrollRegion(params), false
	case 'h', 'l':
		altChanged := false
		if strings.HasPrefix(params, "?") {
			altChanged = f.trackAltScreen(params[1:], final == 'h')
		}
		return seq, altChanged
	case 'H', 'f', 'd': // CUP / HVP / VPA — absolute vertical cursor positioning
		if f.alt.Active {
			return seq, false // alt screen: child owns every row
		}
		return f.clampCursorRow(seq, params), false
	case 'J': // ED (erase in display)
		if f.alt.Active {
			return seq, false // alt screen: child owns every row
		}
		return f.rewriteEraseDisplay(seq, params), false
	default:
		return seq, false
	}
}

// rewriteEraseDisplay keeps ED (erase in display) from wiping the reserved bar,
// which the scroll region does not protect against. CSI 2 J (what `clear`/ncurses
// emit) erases the whole physical screen, so it is rewritten to erase only the
// child region (rows 1..childRows), homing the cursor afterwards to match what
// `clear` (which prefixes CSI H) expects. CSI 0 J / CSI J (erase from cursor to
// end — what fish emits when redrawing a multiline command or stepping through
// history) also erases down through the bar rows; its extent depends on the cursor
// row, which we do not track, so it is forwarded as-is and the bar is flagged for
// an immediate repaint. Erase-to-cursor (CSI 1 J, above the clamped cursor) and
// scrollback (CSI 3 J) cannot touch the visible bar and pass through unchanged.
func (f *AnsiFilter) rewriteEraseDisplay(seq []byte, params string) []byte {
	bottom := f.bottom()
	if bottom == 0 {
		return seq
	}
	switch params {
	case "2":
		// Park at the far column of the last child row (the terminal clamps the
		// column), erase from the top of the display up to there, then home.
		return []byte(fmt.Sprintf("\x1b[%d;999H\x1b[1J\x1b[H", bottom))
	case "", "0":
		f.barClobbered = true
		return seq
	default:
		return seq
	}
}

// clampCursorRow rewrites an absolute vertical move (CUP/HVP `row;col`, VPA `row`)
// whose row lands in the reserved bar area back to the last child row. Full-screen
// programs such as top park the cursor at childRows+1 on exit (e.g. `ESC[60;1H`
// with a 59-row child), relying on the terminal to clamp to its own height. The
// real terminal is taller because of the reserved rows, so an absolute move —
// which the scroll region does not constrain — would otherwise drop the cursor,
// and the returning shell prompt, onto the status bar. Relative moves and line
// feeds stay bounded by the scroll region instead (spec §8.4).
func (f *AnsiFilter) clampCursorRow(seq []byte, params string) []byte {
	bottom := f.bottom()
	if bottom == 0 {
		return seq
	}
	rowStr, rest := params, ""
	if i := strings.IndexByte(params, ';'); i >= 0 {
		rowStr, rest = params[:i], params[i:] // rest keeps ";col"
	}
	if rowStr == "" {
		return seq // row defaults to 1; nothing to clamp
	}
	row, err := strconv.Atoi(rowStr)
	if err != nil || row <= int(bottom) {
		return seq
	}
	return []byte(fmt.Sprintf("\x1b[%d%s%c", bottom, rest, seq[len(seq)-1]))
}

// rewriteScrollRegion enforces that the normal-screen scroll region never
// includes the reserved row(s): a bare `CSI r` becomes `CSI 1 ; bottom r`, and a
// `CSI top ; bottom r` has its bottom clamped to the last child row (spec §8.4).
func (f *AnsiFilter) rewriteScrollRegion(params string) []byte {
	bottom := f.bottom()
	if params == "" {
		return []byte(fmt.Sprintf("\x1b[1;%dr", bottom))
	}
	parts := strings.SplitN(params, ";", 2)
	top, err := strconv.Atoi(parts[0])
	if err != nil || top < 1 {
		top = 1
	}
	bot := int(bottom)
	if len(parts) == 2 {
		if b, err := strconv.Atoi(parts[1]); err == nil && b >= 1 && b <= int(bottom) {
			bot = b
		}
	}
	if top >= bot {
		top = 1
	}
	return []byte(fmt.Sprintf("\x1b[%d;%dr", top, bot))
}

// trackAltScreen toggles alternate-screen state on ?1049/?1047/?47 h/l,
// notifies the handler when the state actually changes, and reports whether a
// change happened.
func (f *AnsiFilter) trackAltScreen(numbers string, set bool) bool {
	for _, ns := range strings.Split(numbers, ";") {
		switch ns {
		case "1049", "1047", "47":
			if f.alt.Active == set {
				return false
			}
			f.alt.Active = set
			if f.onAlt != nil {
				f.onAlt(set)
			}
			return true
		}
	}
	return false
}

// handleOSC consumes whitelisted OSC 777 shell-integration messages (never
// forwarding them) and passes every other OSC through unchanged (spec §9, §17).
func (f *AnsiFilter) handleOSC(seq, out []byte) []byte {
	payload := oscPayload(seq)
	if !strings.HasPrefix(payload, oscShellCode+";") {
		return append(out, seq...) // e.g. OSC 0/2 window title — forward
	}
	// OSC 777 is consumed regardless; whitelisted keys additionally update state.
	body := payload[len(oscShellCode)+1:]
	key, value, ok := parseOSC777(body)
	if !ok {
		f.diag("dropped non-whitelisted or oversized OSC 777 message")
		return out
	}
	f.meta = append(f.meta, event.ShellMeta{Key: key, Value: value})
	return out
}

// oscPayload extracts the content of an OSC sequence between "ESC]" and its
// ST/BEL terminator.
func oscPayload(seq []byte) string {
	body := seq[2:]
	switch {
	case len(body) >= 2 && body[len(body)-2] == escByte && body[len(body)-1] == '\\':
		body = body[:len(body)-2]
	case len(body) >= 1 && body[len(body)-1] == belByte:
		body = body[:len(body)-1]
	case len(body) >= 1 && body[len(body)-1] == escByte:
		body = body[:len(body)-1]
	}
	return string(body)
}
