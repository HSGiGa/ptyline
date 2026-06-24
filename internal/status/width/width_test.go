package width

import "testing"

func TestStringDisplayWidth(t *testing.T) {
	if got := String("中"); got != 2 {
		t.Fatalf("String(中) = %d, want 2 (double-width)", got)
	}
	if got := String("ab"); got != 2 {
		t.Fatalf("String(ab) = %d, want 2", got)
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		side string
		want string
	}{
		{"hello", 10, "right", "hello"},      // fits
		{"hello world", 5, "right", "hell…"}, // trim right
		{"hello world", 5, "left", "…orld"},  // keep tail
		{"hello world", 7, "middle", "hel…rld"},
		{"hello", 3, "none", "hel"}, // hard cut, no ellipsis
		{"中文字", 3, "right", "中…"},   // double-width aware
	}
	for _, c := range cases {
		if got := Truncate(c.in, c.max, c.side); got != c.want {
			t.Fatalf("Truncate(%q,%d,%s) = %q, want %q", c.in, c.max, c.side, got, c.want)
		}
	}
}

func TestPad(t *testing.T) {
	cases := []struct {
		in    string
		width int
		align string
		want  string
	}{
		{"hi", 5, "left", "hi   "},
		{"hi", 5, "right", "   hi"},
		{"hi", 5, "center", " hi  "},
		{"toolong", 3, "left", "toolong"}, // wider than width → unchanged
	}
	for _, c := range cases {
		if got := Pad(c.in, c.width, c.align); got != c.want {
			t.Fatalf("Pad(%q,%d,%s) = %q, want %q", c.in, c.width, c.align, got, c.want)
		}
	}
}
