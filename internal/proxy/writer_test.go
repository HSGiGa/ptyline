package proxy

import (
	"bytes"
	"testing"
)

func TestWriterFlushAndSkipUnchanged(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	w.SetBarRow(30)

	w.RequestRedraw()
	if err := w.FlushBarFrame("hello"); err != nil {
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
	if err := w.FlushBarFrame("hello"); err != nil {
		t.Fatalf("FlushBarFrame: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("identical bar re-flushed %q, want skip", buf.Bytes())
	}
}

// While the alternate screen is active, bar frames are suppressed entirely (§11).
func TestWriterSuppressesBarInAltScreen(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	w.SetBarRow(30)
	w.SetAltActive(true)

	w.RequestRedraw()
	if err := w.FlushBarFrame("hidden"); err != nil {
		t.Fatalf("FlushBarFrame: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("bar painted in alt screen: %q", buf.Bytes())
	}
}

// A second, different frame issued immediately is rate-limited (≤20 Hz) and stays
// pending rather than painting (spec §16).
func TestWriterRateLimits(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	w.SetBarRow(30)

	w.RequestRedraw()
	_ = w.FlushBarFrame("a")
	buf.Reset()

	w.RequestRedraw()
	if err := w.FlushBarFrame("b"); err != nil {
		t.Fatalf("FlushBarFrame: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("rate limit not enforced: wrote %q", buf.Bytes())
	}
}
