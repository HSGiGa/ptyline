package proxy

import (
	"bytes"
	"testing"
)

func TestWriterFlushAndSkipUnchanged(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	w.SetBarRows(30, 1)

	w.RequestRedraw()
	if err := w.FlushBarFrame([]string{"hello"}); err != nil {
		t.Fatalf("FlushBarFrame: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected a bar frame to be written")
	}
	// A bar frame must never carry a trailing newline (it would scroll the bar
	// into scrollback — spec §8.6).
	if bytes.Contains(buf.Bytes(), []byte{'\n'}) {
		t.Fatalf("bar frame contains a newline: %q", buf.Bytes())
	}

	// Re-requesting the same content is a no-op (spec §16).
	buf.Reset()
	w.RequestRedraw()
	if err := w.FlushBarFrame([]string{"hello"}); err != nil {
		t.Fatalf("FlushBarFrame: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("identical bar re-flushed %q, want skip", buf.Bytes())
	}
}

// When fewer rows fit than the bar has, the bottom rows (content nearest the
// prompt) are kept and the top decorative rows are dropped.
func TestWriterShortTerminalKeepsBottomRows(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	w.SetBarRows(30, 1) // only one row fits

	w.RequestRedraw()
	if err := w.FlushBarFrame([]string{"TOP-border", "BOTTOM-content"}); err != nil {
		t.Fatalf("FlushBarFrame: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("BOTTOM-content")) {
		t.Fatalf("bottom row dropped: %q", buf.Bytes())
	}
	if bytes.Contains(buf.Bytes(), []byte("TOP-border")) {
		t.Fatalf("top row painted despite no room: %q", buf.Bytes())
	}
}

// While the alternate screen is active, bar frames are suppressed entirely (§11).
func TestWriterSuppressesBarInAltScreen(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	w.SetBarRows(30, 1)
	w.SetAltActive(true)

	w.RequestRedraw()
	if err := w.FlushBarFrame([]string{"hidden"}); err != nil {
		t.Fatalf("FlushBarFrame: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("bar painted in alt screen: %q", buf.Bytes())
	}
}

func TestWriterForcesRedrawAfterAltScreen(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	w.SetBarRows(30, 1)

	w.RequestRedraw()
	if err := w.FlushBarFrame([]string{"bar"}); err != nil {
		t.Fatalf("initial FlushBarFrame: %v", err)
	}
	buf.Reset()

	w.SetAltActive(true)
	w.SetAltActive(false)
	w.RequestRedraw()
	if err := w.FlushBarFrame([]string{"bar"}); err != nil {
		t.Fatalf("post-alt FlushBarFrame: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected bar to repaint after leaving alt screen")
	}
}

// A second, different frame issued immediately is rate-limited (≤20 Hz) and stays
// pending rather than painting (spec §16).
func TestWriterRateLimits(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	w.SetBarRows(30, 1)

	w.RequestRedraw()
	_ = w.FlushBarFrame([]string{"a"})
	buf.Reset()

	w.RequestRedraw()
	if err := w.FlushBarFrame([]string{"b"}); err != nil {
		t.Fatalf("FlushBarFrame: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("rate limit not enforced: wrote %q", buf.Bytes())
	}
}
