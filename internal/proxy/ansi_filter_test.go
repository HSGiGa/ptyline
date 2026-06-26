package proxy

import (
	"testing"

	"github.com/hsgiga/ptyline/internal/reserved"
)

// newFilter builds a filter for a 30-row terminal (child rows = 29, bottom = 29).
func newFilter(onMeta func(k, v string)) *AnsiFilter {
	f := NewAnsiFilter(reserved.Default(), onMeta)
	f.SetRows(30)
	return f
}

// In the normal screen a bare DECSTBM reset becomes 1;bottom, and a region whose
// bottom overlaps the reserved row is clamped (spec §8.4).
func TestNormalScreenScrollRegionRewrite(t *testing.T) {
	cases := []struct{ in, want string }{
		{"\x1b[r", "\x1b[1;29r"},     // bare → full child area
		{"\x1b[1;30r", "\x1b[1;29r"}, // bottom clamped off the reserved row
		{"\x1b[5;20r", "\x1b[5;20r"}, // already inside child area → unchanged
	}
	for _, c := range cases {
		f := newFilter(nil)
		if got := string(f.Filter([]byte(c.in))); got != c.want {
			t.Fatalf("Filter(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// In the alternate screen the child owns every row, so the filter must NOT clamp
// scroll margins (spec §8.4, §11).
func TestAltScreenScrollRegionPassesThrough(t *testing.T) {
	f := newFilter(nil)
	f.Filter([]byte("\x1b[?1049h")) // enter alt screen
	if got := string(f.Filter([]byte("\x1b[1;30r"))); got != "\x1b[1;30r" {
		t.Fatalf("alt-screen DECSTBM = %q, want pass-through", got)
	}
}

func TestAltScreenToggleHandler(t *testing.T) {
	f := newFilter(nil)
	var active bool
	var calls int
	f.SetAltHandler(func(a bool) { active = a; calls++ })

	f.Filter([]byte("\x1b[?1049h"))
	if !active || !f.AltActive() {
		t.Fatal("expected alt screen active after ?1049h")
	}
	f.Filter([]byte("\x1b[?1049l"))
	if active || f.AltActive() {
		t.Fatal("expected alt screen inactive after ?1049l")
	}
	if calls != 2 {
		t.Fatalf("alt handler called %d times, want 2", calls)
	}
}

func TestAltScreenToggleVariants(t *testing.T) {
	for _, code := range []string{"?1049", "?1047", "?47"} {
		t.Run(code, func(t *testing.T) {
			f := newFilter(nil)
			var calls int
			f.SetAltHandler(func(bool) { calls++ })

			f.Filter([]byte("\x1b[" + code + "h"))
			f.Filter([]byte("\x1b[" + code + "h")) // duplicate enter is ignored
			if !f.AltActive() {
				t.Fatalf("%s enter did not activate alt screen", code)
			}
			f.Filter([]byte("\x1b[" + code + "l"))
			f.Filter([]byte("\x1b[" + code + "l")) // duplicate leave is ignored
			if f.AltActive() {
				t.Fatalf("%s leave did not deactivate alt screen", code)
			}
			if calls != 2 {
				t.Fatalf("%s handler calls = %d, want 2", code, calls)
			}
		})
	}
}

// A sequence split across two reads must be reassembled via the tail buffer.
func TestPartialSequenceReassembly(t *testing.T) {
	f := newFilter(nil)
	if got := f.Filter([]byte("\x1b[1;3")); len(got) != 0 {
		t.Fatalf("incomplete sequence produced output %q, want buffered", got)
	}
	if got := string(f.Filter([]byte("0r"))); got != "\x1b[1;29r" {
		t.Fatalf("reassembled Filter = %q, want \\x1b[1;29r", got)
	}
}

// Whitelisted OSC 777 is consumed (never forwarded) and reported via onMeta.
func TestOSC777Consumed(t *testing.T) {
	var k, v string
	f := newFilter(func(key, val string) { k, v = key, val })
	if got := f.Filter([]byte("\x1b]777;cwd=/tmp\x07")); len(got) != 0 {
		t.Fatalf("OSC 777 forwarded %q, want consumed", got)
	}
	if k != "cwd" || v != "/tmp" {
		t.Fatalf("onMeta got (%q,%q), want (cwd,/tmp)", k, v)
	}
	meta := f.DrainMeta()
	if len(meta) != 1 || meta[0].Key != "cwd" || meta[0].Value != "/tmp" {
		t.Fatalf("DrainMeta got %+v, want cwd=/tmp", meta)
	}
	if meta := f.DrainMeta(); len(meta) != 0 {
		t.Fatalf("DrainMeta second call got %+v, want empty", meta)
	}
}

// Ordinary OSC (e.g. window title, OSC 0) passes through unchanged.
func TestOrdinaryOSCPassThrough(t *testing.T) {
	f := newFilter(nil)
	in := "\x1b]0;my title\x07"
	if got := string(f.Filter([]byte(in))); got != in {
		t.Fatalf("OSC 0 = %q, want pass-through", got)
	}
}
