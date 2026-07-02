package proxy

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hsgiga/ptyline/internal/terminal"
)

// WriteChildFrame brackets the child output and the bar repaint in ONE synchronized
// update so a child erase that wiped the bar and the repaint render as one frame.
func TestWriteChildFrameIsAtomic(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	w.SetBarRows(30, 1)

	child := []byte("cmd\x1b[0J") // child output that erases to end (clobbers the bar)
	if err := w.WriteChildFrame(child, []string{"bar"}, false); err != nil {
		t.Fatalf("WriteChildFrame: %v", err)
	}
	out := buf.String()

	// Exactly one synchronized-update span, opened before the child bytes and closed
	// after the bar repaint — so the terminal never renders the cleared bar alone.
	if strings.Count(out, terminal.BeginSyncUpdate) != 1 || strings.Count(out, terminal.EndSyncUpdate) != 1 {
		t.Fatalf("want one sync span, got %q", out)
	}
	begin := strings.Index(out, terminal.BeginSyncUpdate)
	childAt := strings.Index(out, "cmd\x1b[0J")
	barAt := strings.Index(out, "bar")
	end := strings.Index(out, terminal.EndSyncUpdate)
	if !(begin < childAt && childAt < barAt && barAt < end) {
		t.Fatalf("order should be sync-begin < child < bar < sync-end, got %q", out)
	}

	// Having painted "bar", an identical subsequent flush is a no-op.
	buf.Reset()
	w.RequestRedraw()
	if err := w.FlushBarFrame([]string{"bar"}, false); err != nil {
		t.Fatalf("FlushBarFrame: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("WriteChildFrame should have recorded the painted bar; re-flush wrote %q", buf.Bytes())
	}
}

// With no reserved bar (or on the alt screen) WriteChildFrame degrades to a plain
// child write with no synchronized-update wrapping.
func TestWriteChildFrameNoBarPlainWrite(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	// barTop/barCount unset → no bar.
	if err := w.WriteChildFrame([]byte("cmd"), []string{"bar"}, false); err != nil {
		t.Fatalf("WriteChildFrame: %v", err)
	}
	if out := buf.String(); out != "cmd" {
		t.Fatalf("no-bar write = %q, want plain %q", out, "cmd")
	}
}

func TestWriterFlushAndSkipUnchanged(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	w.SetBarRows(30, 1)

	w.RequestRedraw()
	if err := w.FlushBarFrame([]string{"hello"}, false); err != nil {
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
	if err := w.FlushBarFrame([]string{"hello"}, false); err != nil {
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
	if err := w.FlushBarFrame([]string{"TOP-border", "BOTTOM-content"}, false); err != nil {
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
	w.OnAltEnter()

	w.RequestRedraw()
	// Callers pass alt=true when the filter reports alt active.
	if err := w.FlushBarFrame([]string{"hidden"}, true); err != nil {
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
	if err := w.FlushBarFrame([]string{"bar"}, false); err != nil {
		t.Fatalf("initial FlushBarFrame: %v", err)
	}
	buf.Reset()

	// Simulate: enter alt (clears bar state), then leave alt (caller passes false).
	w.OnAltEnter()
	w.RequestRedraw()
	if err := w.FlushBarFrame([]string{"bar"}, false); err != nil {
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
	_ = w.FlushBarFrame([]string{"a"}, false)
	buf.Reset()

	w.RequestRedraw()
	if err := w.FlushBarFrame([]string{"b"}, false); err != nil {
		t.Fatalf("FlushBarFrame: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("rate limit not enforced: wrote %q", buf.Bytes())
	}
}

func TestWriterLazyFlushSkipsRenderWhenRateLimited(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalWriter(&buf)
	w.SetBarRows(30, 1)

	w.RequestRedraw()
	_ = w.FlushBarFrameLazy(func() []string { return []string{"a"} }, false)
	buf.Reset()

	called := false
	w.RequestRedraw()
	if err := w.FlushBarFrameLazy(func() []string {
		called = true
		return []string{"b"}
	}, false); err != nil {
		t.Fatalf("FlushBarFrameLazy: %v", err)
	}
	if called {
		t.Fatal("lazy render called while frame was rate-limited")
	}
	if buf.Len() != 0 {
		t.Fatalf("rate-limited lazy frame wrote %q", buf.Bytes())
	}
}
