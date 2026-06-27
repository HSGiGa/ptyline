package shellcolors

import (
	"testing"
)

func TestParseColors(t *testing.T) {
	cases := []struct {
		input string
		want  map[string]string
	}{
		{
			"cwd=green;host=normal;status=red",
			map[string]string{"cwd": "green", "host": "normal", "status": "red"},
		},
		{
			"cwd=green --bold;host=normal",
			map[string]string{"cwd": "green --bold", "host": "normal"},
		},
		{
			"",
			map[string]string{},
		},
		{
			"noequals",
			map[string]string{},
		},
		{
			"cwd=",
			map[string]string{"cwd": ""},
		},
	}
	for _, c := range cases {
		got := ParseColors(c.input)
		if len(got) != len(c.want) {
			t.Errorf("ParseColors(%q) = %v, want %v", c.input, got, c.want)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("ParseColors(%q)[%q] = %q, want %q", c.input, k, got[k], v)
			}
		}
	}
}

func TestParseColorAttr(t *testing.T) {
	cases := []struct {
		input     string
		wantColor string
		wantBold  bool
	}{
		{"green", "green", false},
		{"green --bold", "green", true},
		{"brred", "brightred", false},
		{"brgreen --bold", "brightgreen", true},
		{"brcyan", "brightcyan", false},
		{"normal", "", false},
		{"", "", false},
		{"--bold", "", true},
		{"cyan --italics --bold", "cyan", true},
		{"white --background=brblack", "white", false},
		{"brblack", "brightblack", false},
	}
	for _, c := range cases {
		gotColor, gotBold := ParseColorAttr(c.input)
		if gotColor != c.wantColor || gotBold != c.wantBold {
			t.Errorf("ParseColorAttr(%q) = (%q, %v), want (%q, %v)",
				c.input, gotColor, gotBold, c.wantColor, c.wantBold)
		}
	}
}

func TestParseToStyles(t *testing.T) {
	value := "cwd=green;host=normal;status=red;command=normal;user=brgreen;unknown=blue"
	styles := ParseToStyles(value)

	// known mappings
	if s, ok := styles["cwd"]; !ok || s.FG != "green" || s.Bold {
		t.Errorf("cwd style = %+v, want FG=green Bold=false", s)
	}
	if s, ok := styles["hostname"]; !ok || s.FG != "" || s.Bold {
		t.Errorf("hostname style = %+v, want FG='' Bold=false (normal)", s)
	}
	if s, ok := styles["exit_code"]; !ok || s.FG != "red" || s.Bold {
		t.Errorf("exit_code style = %+v, want FG=red Bold=false", s)
	}
	if s, ok := styles["command"]; !ok || s.FG != "" || s.Bold {
		t.Errorf("command style = %+v, want FG='' Bold=false (normal)", s)
	}

	// "user" and "unknown" have no module mapping — must be absent
	if _, ok := styles["user"]; ok {
		t.Error("user element should not produce a style (no module mapping)")
	}
	if _, ok := styles["unknown"]; ok {
		t.Error("unknown element should be silently dropped")
	}
}

func TestParseToStylesBold(t *testing.T) {
	styles := ParseToStyles("cwd=green --bold;host=brgreen --bold")
	if s := styles["cwd"]; s.FG != "green" || !s.Bold {
		t.Errorf("cwd bold style = %+v, want FG=green Bold=true", s)
	}
	if s := styles["hostname"]; s.FG != "brightgreen" || !s.Bold {
		t.Errorf("hostname bold style = %+v, want FG=brightgreen Bold=true", s)
	}
}
