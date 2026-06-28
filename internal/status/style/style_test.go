package style

import (
	"strings"
	"testing"

	"github.com/hsgiga/ptyline/internal/status/theme"
)

// No-color mode (nil theme) emits plain text with padding and separators, never
// an escape byte (spec §20.14, §20.15).
func TestApplyNoColorPlain(t *testing.T) {
	s := Style{FG: "accent", BG: "base.bg", Bold: true, PaddingLeft: 1, PaddingRight: 1, LeftCap: "[", RightCap: "]"}
	got := s.Apply("hi", nil)
	if want := "[ hi ]"; got != want {
		t.Fatalf("Apply no-color = %q, want %q", got, want)
	}
	if strings.Contains(got, "\x1b") {
		t.Fatalf("no-color output leaked an escape: %q", got)
	}
}

// Truecolor mode emits fg, bg, attributes, padded content, then a reset.
func TestApplyTrueColorGolden(t *testing.T) {
	th := theme.Default(theme.TrueColor)
	// accent=brightcyan RGB{0,255,255}; error=standard red RGB{205,0,0}
	s := Style{FG: "accent", BG: "error", Bold: true, PaddingLeft: 1, PaddingRight: 1}
	want := "\x1b[38;2;0;255;255m" + "\x1b[48;2;205;0;0m" + "\x1b[1m" + " host " + theme.Reset
	if got := s.Apply("host", th); got != want {
		t.Fatalf("Apply truecolor =\n %q\nwant\n %q", got, want)
	}
}

// A style always resets, so attributes never bleed into following output.
func TestApplyAlwaysResets(t *testing.T) {
	th := theme.Default(theme.TrueColor)
	got := Style{FG: "accent"}.Apply("x", th)
	if !strings.HasSuffix(got, theme.Reset) {
		t.Fatalf("styled output must end with reset: %q", got)
	}
}
