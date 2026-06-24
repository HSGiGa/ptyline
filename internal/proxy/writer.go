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
	barRow       uint16    // 1-based row the bar is drawn on
	lastBar      string    // last bar line written, to skip no-op redraws (spec §16)
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
		w.lastBar = ""
	}
}

// SetBarRow records the 1-based terminal row the bar is drawn on (updated on
// resize). A new row invalidates the no-op cache so the next flush always paints.
func (w *TerminalWriter) SetBarRow(row uint16) {
	if row != w.barRow {
		w.barRow = row
		w.lastBar = ""
	}
}

// RequestRedraw marks that the bar should be redrawn at the next safe boundary.
func (w *TerminalWriter) RequestRedraw() { w.pendingFrame = true }

// InvalidateBar marks the terminal copy of the bar as unknown. Child output may
// have cleared or overwritten the last row (for example fish's `clear`), so the
// next redraw must repaint even when the rendered line itself is unchanged.
func (w *TerminalWriter) InvalidateBar() { w.lastBar = "" }

// ClearBar removes the rendered bar while preserving the user's cursor
// position. It is called during wrapper shutdown before terminal restoration.
func (w *TerminalWriter) ClearBar() error {
	if w.barRow == 0 {
		return nil
	}
	frame := terminal.BeginSyncUpdate +
		terminal.SaveCursor +
		terminal.CursorTo(w.barRow, 1) +
		terminal.ClearLine +
		terminal.ResetAttrs +
		terminal.RestoreCursor +
		terminal.EndSyncUpdate
	return w.writeAll([]byte(frame))
}

// FlushBarFrame emits a complete bar frame if one is pending, the alternate
// screen is inactive, the rate limit allows it, and the content changed. The
// frame uses absolute positioning and carries NO trailing newline, which would
// scroll the bar into history (spec §8.6, docs/terminal-safety.md).
func (w *TerminalWriter) FlushBarFrame(line string) error {
	if w.altActive || !w.pendingFrame || w.barRow == 0 {
		return nil
	}
	if line == w.lastBar {
		w.pendingFrame = false
		return nil
	}
	if !w.lastBarAt.IsZero() && time.Since(w.lastBarAt) < time.Second/maxBarRedrawHz {
		return nil // rate-limited; stay pending for the next boundary
	}
	frame := terminal.BeginSyncUpdate +
		terminal.SaveCursor +
		terminal.CursorTo(w.barRow, 1) +
		terminal.ClearLine +
		line +
		terminal.ResetAttrs +
		terminal.RestoreCursor +
		terminal.EndSyncUpdate
	if err := w.writeAll([]byte(frame)); err != nil {
		return err
	}
	w.lastBar = line
	w.lastBarAt = time.Now()
	w.pendingFrame = false
	return nil
}
