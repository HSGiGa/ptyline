package terminal

import (
	"bytes"
	"testing"

	"github.com/hsgiga/ptyline/internal/reserved"
)

func TestCursorTo(t *testing.T) {
	if got := CursorTo(5, 1); got != "\x1b[5;1H" {
		t.Fatalf("CursorTo(5,1) = %q", got)
	}
}

func TestSetScrollRegion(t *testing.T) {
	if got := SetScrollRegion(1, 29); got != "\x1b[1;29r" {
		t.Fatalf("SetScrollRegion(1,29) = %q", got)
	}
}

// ApplyScrollRegion must protect the reserved row: a 30-row terminal scrolls only
// rows 1..29 (spec §6).
func TestApplyScrollRegionExcludesReservedRow(t *testing.T) {
	var buf bytes.Buffer
	c := New(nil, &buf)
	c.ApplyScrollRegion(Size{Cols: 80, Rows: 30}, reserved.Default())
	// The region is 1..29, wrapped in cursor save/restore so DECSTBM does not
	// home the cursor.
	if got, want := buf.String(), SaveCursor+"\x1b[1;29r"+RestoreCursor; got != want {
		t.Fatalf("ApplyScrollRegion wrote %q, want %q", got, want)
	}
}

func TestApplyScrollRegionAtChildBottomMovesCursorOutOfBar(t *testing.T) {
	var buf bytes.Buffer
	c := New(nil, &buf)
	c.ApplyScrollRegionAtChildBottom(Size{Cols: 80, Rows: 30}, reserved.Default())
	if got, want := buf.String(), "\x1b[1;29r"+CursorTo(29, 1); got != want {
		t.Fatalf("ApplyScrollRegionAtChildBottom wrote %q, want %q", got, want)
	}
}

// SuspendRaw and ResumeRaw do not emit any escape sequences; they only toggle
// the raw-mode state. Calling them on a non-tty (as in tests) is graceful.
func TestSuspendResumeRawNoOutput(t *testing.T) {
	var buf bytes.Buffer
	c := New(nil, &buf)
	// On a non-tty these return errors but must not panic.
	_ = c.SuspendRaw()
	_ = c.ResumeRaw()
	if buf.Len() != 0 {
		t.Fatalf("SuspendRaw/ResumeRaw wrote %q, want empty", buf.String())
	}
}

// Restore emits the exact cleanup order and is idempotent (spec §8.1, §15).
func TestRestoreOrderIdempotent(t *testing.T) {
	var buf bytes.Buffer
	c := New(nil, &buf)
	want := ResetScrollRegion + ResetAttrs + RestoreCursor + ShowCursor
	if err := c.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := buf.String(); got != want {
		t.Fatalf("Restore wrote %q, want %q", got, want)
	}
	buf.Reset()
	if err := c.Restore(); err != nil {
		t.Fatalf("second Restore: %v", err)
	}
	if got := buf.String(); got != want {
		t.Fatalf("second Restore wrote %q, want identical output", got)
	}
}
