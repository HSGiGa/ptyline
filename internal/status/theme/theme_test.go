package theme

import "testing"

// No-color mode emits nothing, regardless of the reference.
func TestNoColorEmitsNothing(t *testing.T) {
	th := Default(NoColor)
	for _, ref := range []string{"accent", "base.bg", "#ff0000", "red", "bogus"} {
		if got := th.FG(ref); got != "" {
			t.Errorf("FG(%q) in no-color = %q, want empty", ref, got)
		}
		if got := th.BG(ref); got != "" {
			t.Errorf("BG(%q) in no-color = %q, want empty", ref, got)
		}
	}
}

// Truecolor renders tokens and hex literals as 24-bit SGR; unknown refs are empty.
func TestTrueColorResolution(t *testing.T) {
	th := Default(TrueColor)
	cases := []struct{ ref, want string }{
		{"base.bg", ""},                   // not in terminal-native palette → no bg emitted
		{"#ff0000", "\x1b[48;2;255;0;0m"}, // BG layer
		{"bogus", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := th.BG(c.ref); got != c.want {
			t.Errorf("BG(%q) = %q, want %q", c.ref, got, c.want)
		}
	}
	// accent = brightcyan (ANSI 14) = RGB{0,255,255}
	if got, want := th.FG("accent"), "\x1b[38;2;0;255;255m"; got != want {
		t.Errorf("FG(accent) = %q, want %q", got, want)
	}
}

// Named colors map to the 16-color ANSI codes (fg 30–37/90–97, bg +10).
func TestColor16NamedCodes(t *testing.T) {
	th := New(Color16, map[string]RGB{})
	if got, want := th.FG("red"), "\x1b[31m"; got != want {
		t.Errorf("FG(red) = %q, want %q", got, want)
	}
	if got, want := th.BG("brightblue"), "\x1b[104m"; got != want {
		t.Errorf("BG(brightblue) = %q, want %q", got, want)
	}
}

// 256-color mode renders a palette index.
func TestColor256Form(t *testing.T) {
	th := Default(Color256)
	got := th.FG("accent")
	if len(got) < 7 || got[:5] != "\x1b[38;" || got[len(got)-1] != 'm' {
		t.Errorf("FG(accent) 256 form unexpected: %q", got)
	}
}
