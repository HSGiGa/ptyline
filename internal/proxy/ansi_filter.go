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
	area   reserved.Area
	rows   uint16 // current real-terminal rows
	alt    AltScreenState
	tail   []byte // buffered incomplete escape sequence
	meta   []event.ShellMeta
	onAlt  func(active bool)
	onDiag func(msg string)
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
	if len(f.tail) > 0 {
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
			if len(rest) > maxBufferedCSI {
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
		out = f.handleSequence(seq, out)
		i += n
	}
	return out
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
// forward (possibly rewritten, possibly nothing) to out.
func (f *AnsiFilter) handleSequence(seq, out []byte) []byte {
	if len(seq) < 2 {
		return append(out, seq...)
	}
	switch seq[1] {
	case '[':
		return append(out, f.handleCSI(seq)...)
	case ']':
		return f.handleOSC(seq, out)
	default:
		return append(out, seq...)
	}
}

// handleCSI rewrites/clamps DECSTBM and tracks alt-screen toggles. It returns the
// bytes to forward (unchanged for everything it does not touch).
func (f *AnsiFilter) handleCSI(seq []byte) []byte {
	final := seq[len(seq)-1]
	params := string(seq[2 : len(seq)-1]) // between "ESC[" and the final byte

	switch final {
	case 'r': // DECSTBM (set scroll region)
		if f.alt.Active || strings.HasPrefix(params, "?") {
			return seq // alt screen: child owns every row; private 'r' is unrelated
		}
		return f.rewriteScrollRegion(params)
	case 'h', 'l':
		if strings.HasPrefix(params, "?") {
			f.trackAltScreen(params[1:], final == 'h')
		}
		return seq
	default:
		return seq
	}
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

// trackAltScreen toggles alternate-screen state on ?1049/?1047/?47 h/l and
// notifies the handler when the state actually changes (spec §11).
func (f *AnsiFilter) trackAltScreen(numbers string, set bool) {
	for _, ns := range strings.Split(numbers, ";") {
		switch ns {
		case "1049", "1047", "47":
			if f.alt.Active == set {
				return
			}
			f.alt.Active = set
			if f.onAlt != nil {
				f.onAlt(set)
			}
			return
		}
	}
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
