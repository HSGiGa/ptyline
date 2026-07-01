package proxy

import (
	"errors"
	"io"
	"syscall"
	"time"

	"github.com/hsgiga/ptyline/internal/terminal"
)

// maxBarRedrawHz bounds how often a bar frame is emitted (spec §16: "maximum bar
// redraw rate: 20 Hz").
const maxBarRedrawHz = 20

// TerminalWriter is the single serialized writer for everything that reaches the
// real terminal: filtered child output and complete bar frames. Serializing here
// is a core invariant (spec §8.3, §11):
//
//   - a bar frame is NEVER inserted into the middle of a child-output write;
//   - child bytes are never dropped, duplicated, or reordered (short and
//     interrupted writes are retried);
//   - a redraw is emitted only at a safe event-loop boundary and is rate-limited;
//   - while the alternate screen is active, bar frames are suppressed entirely.
//
// All terminal writes in the program must go through one TerminalWriter instance.
// It is only ever touched from the event-loop goroutine, so it needs no lock.
type TerminalWriter struct {
	out          io.Writer
	altActive    bool
	barTop       uint16    // 1-based first bar row
	barCount     int       // number of reserved bar rows
	lastBars     []string  // last bar lines written, to skip no-op redraws (spec §16)
	lastBarAt    time.Time // for rate limiting
	pendingFrame bool      // a redraw was requested but deferred to a safe boundary
}

// NewTerminalWriter wraps the real-terminal output.
func NewTerminalWriter(out io.Writer) *TerminalWriter {
	return &TerminalWriter{out: out}
}

// writeAll drains b, retrying short writes and EINTR (spec §8.3, §16). On a
// broken pipe it stops — the terminal is gone.
func (w *TerminalWriter) writeAll(b []byte) error {
	for len(b) > 0 {
		n, err := w.out.Write(b)
		b = b[n:]
		if err == nil {
			continue
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		return err
	}
	return nil
}

// WriteChild writes filtered child output verbatim, fully draining b.
func (w *TerminalWriter) WriteChild(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	return w.writeAll(b)
}

// SetAltActive toggles alternate-screen mode. While active, bar frames are
// suppressed and any pending redraw is dropped (spec §11).
func (w *TerminalWriter) SetAltActive(active bool) {
	w.altActive = active
	if active {
		w.pendingFrame = false
		// Force a redraw on return to the normal screen.
		w.lastBars = nil
		w.lastBarAt = time.Time{}
	}
}

// SetBarRows records the 1-based first bar row and how many rows the bar spans
// (updated on resize). A change invalidates the no-op cache so the next flush
// always paints.
func (w *TerminalWriter) SetBarRows(top uint16, count int) {
	if top != w.barTop || count != w.barCount {
		w.barTop = top
		w.barCount = count
		w.lastBars = nil
	}
}

// RequestRedraw marks that the bar should be redrawn at the next safe boundary.
func (w *TerminalWriter) RequestRedraw() { w.pendingFrame = true }

// InvalidateBar marks the terminal copy of the bar as unknown. Child output may
// have cleared or overwritten the reserved rows (for example fish's `clear` or a
// cursor-to-end erase), so the next redraw must repaint even when the rendered
// lines are unchanged. The rate-limit clock is also reset so the restore is not
// deferred — a missing bar must not wait up to the redraw interval to reappear.
func (w *TerminalWriter) InvalidateBar() {
	w.lastBars = nil
	w.lastBarAt = time.Time{}
}

// ClearBar removes the rendered bar (all reserved rows) while preserving the
// user's cursor position. Called during wrapper shutdown before restoration.
func (w *TerminalWriter) ClearBar() error {
	if w.barTop == 0 || w.barCount == 0 {
		return nil
	}
	frame := terminal.BeginSyncUpdate + terminal.SaveCursor
	for i := 0; i < w.barCount; i++ {
		frame += terminal.CursorTo(w.barTop+uint16(i), 1) + terminal.ClearLine
	}
	frame += terminal.ResetAttrs + terminal.RestoreCursor + terminal.EndSyncUpdate
	return w.writeAll([]byte(frame))
}

// FlushBarFrame emits a complete bar frame (all reserved rows) if one is pending,
// the alternate screen is inactive, the rate limit allows it, and the content
// changed. The frame uses absolute positioning and carries NO trailing newline,
// which would scroll the bar into history.
// When the terminal is too short to show every row (barCount < len(lines)), the
// BOTTOM rows are kept — the content nearest the prompt — and the top decorative
// rows are dropped, so a short terminal never paints past its last row.
func (w *TerminalWriter) FlushBarFrame(lines []string) error {
	if !w.readyForBarFrame() {
		return nil
	}
	return w.flushBarFrame(lines)
}

// FlushBarFrameLazy is the hot-path variant of FlushBarFrame: render is called
// only when a frame is actually eligible to be emitted. This keeps high-rate PTY
// output from paying layout/render costs for frames that the rate limiter will
// suppress.
func (w *TerminalWriter) FlushBarFrameLazy(render func() []string) error {
	if !w.readyForBarFrame() {
		return nil
	}
	return w.flushBarFrame(render())
}

func (w *TerminalWriter) readyForBarFrame() bool {
	if w.altActive || !w.pendingFrame || w.barTop == 0 || w.barCount == 0 {
		return false
	}
	if !w.lastBarAt.IsZero() && time.Since(w.lastBarAt) < time.Second/maxBarRedrawHz {
		return false
	}
	return true
}

func (w *TerminalWriter) flushBarFrame(lines []string) error {
	if equalLines(lines, w.lastBars) {
		w.pendingFrame = false
		w.lastBarAt = time.Now()
		return nil
	}
	frame := terminal.BeginSyncUpdate + w.barPaintBody(lines) + terminal.EndSyncUpdate
	if err := w.writeAll([]byte(frame)); err != nil {
		return err
	}
	w.markPainted(lines)
	return nil
}

// WriteChildFrame writes child output and an immediate bar repaint inside ONE
// synchronized update. It is used when the child output erased the reserved rows
// (a cursor-to-end CSI 0 J, e.g. fish redrawing a multiline command or stepping
// through history): forwarding the erase and repainting in separate terminal
// frames makes the bar blink blank for a frame, so the two are bracketed together
// and the bar never renders empty. With no bar (or on the alt screen) it degrades
// to a plain child write.
func (w *TerminalWriter) WriteChildFrame(child []byte, lines []string) error {
	if w.altActive || w.barTop == 0 || w.barCount == 0 {
		return w.WriteChild(child)
	}
	if err := w.writeAll([]byte(terminal.BeginSyncUpdate)); err != nil {
		return err
	}
	if err := w.writeAll(child); err != nil {
		return err
	}
	if err := w.writeAll([]byte(w.barPaintBody(lines) + terminal.EndSyncUpdate)); err != nil {
		return err
	}
	w.markPainted(lines)
	return nil
}

// barPaintBody renders the reserved rows between a saved and restored cursor,
// without the surrounding synchronized-update markers so callers can compose it.
// When the terminal is too short (barCount < len(lines)), the BOTTOM rows are kept
// — the content nearest the prompt — and the top decorative rows are dropped.
func (w *TerminalWriter) barPaintBody(lines []string) string {
	start := len(lines) - w.barCount
	if start < 0 {
		start = 0
	}
	body := terminal.SaveCursor
	for i := 0; i < w.barCount && start+i < len(lines); i++ {
		body += terminal.CursorTo(w.barTop+uint16(i), 1) + terminal.ClearLine + lines[start+i] + terminal.ResetAttrs
	}
	return body + terminal.RestoreCursor
}

func (w *TerminalWriter) markPainted(lines []string) {
	w.lastBars = append(w.lastBars[:0], lines...)
	w.lastBarAt = time.Now()
	w.pendingFrame = false
}

// equalLines reports whether two rendered bar frames are identical.
func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
