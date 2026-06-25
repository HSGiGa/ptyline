package runtimeenv

import "testing"

func TestDetectColor(t *testing.T) {
	lookup := func(env map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	}
	cases := []struct {
		name string
		env  map[string]string
		want ColorLevel
	}{
		{"no_color wins over truecolor", map[string]string{"NO_COLOR": "1", "COLORTERM": "truecolor"}, ColorNone},
		{"empty no_color is ignored", map[string]string{"NO_COLOR": "", "COLORTERM": "truecolor"}, ColorTrue},
		{"dumb terminal", map[string]string{"TERM": "dumb"}, ColorNone},
		{"truecolor via colorterm", map[string]string{"TERM": "xterm", "COLORTERM": "24bit"}, ColorTrue},
		{"256 via term", map[string]string{"TERM": "xterm-256color"}, Color256},
		{"basic via term", map[string]string{"TERM": "xterm"}, ColorBasic},
		{"no term, no color", map[string]string{}, ColorNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detectColor(lookup(c.env)); got != c.want {
				t.Errorf("detectColor(%v) = %v, want %v", c.env, got, c.want)
			}
		})
	}
}
