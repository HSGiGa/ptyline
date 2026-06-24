package proxy

import (
	"io"
	"time"
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
type TerminalWriter struct {
	out          io.Writer
	altActive    bool
	lastBar      string    // last bar line written, to skip no-op redraws (spec §16)
	lastBarAt    time.Time // for rate limiting
	pendingFrame bool      // a redraw was requested but deferred to a safe boundary
}

// NewTerminalWriter wraps the real-terminal output.
func NewTerminalWriter(out io.Writer) *TerminalWriter {
	return &TerminalWriter{out: out}
}

// WriteChild writes filtered child output verbatim. It must fully drain b,
// retrying short/interrupted writes (spec §8.3, §16).
//
// TODO scaffold (plan 05): loop on io.Writer.Write handling n<len and EINTR.
func (w *TerminalWriter) WriteChild(b []byte) error {
	_, err := w.out.Write(b)
	return err
}

// SetAltActive toggles alternate-screen mode. While active, bar frames are
// suppressed (spec §11).
func (w *TerminalWriter) SetAltActive(active bool) { w.altActive = active }

// RequestRedraw marks that the bar should be redrawn at the next safe boundary.
func (w *TerminalWriter) RequestRedraw() { w.pendingFrame = true }

// FlushBarFrame emits a complete bar frame if one is pending, the alternate
// screen is inactive, the rate limit allows it, and the content changed.
//
// TODO scaffold (plan 05/09): render the frame with absolute positioning (no
// trailing newline — see docs/terminal-safety.md) and write it atomically here.
func (w *TerminalWriter) FlushBarFrame(line string) error {
	if w.altActive || !w.pendingFrame {
		return nil
	}
	if line == w.lastBar {
		w.pendingFrame = false
		return nil
	}
	if !w.lastBarAt.IsZero() && time.Since(w.lastBarAt) < time.Second/maxBarRedrawHz {
		return nil // rate-limited; stay pending for the next boundary
	}
	// TODO scaffold: write the absolute-positioned frame for `line`.
	w.lastBar = line
	w.lastBarAt = time.Now()
	w.pendingFrame = false
	return nil
}
