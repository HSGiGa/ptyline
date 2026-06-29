package modules

import "testing"

func TestFormatPercent(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want string
	}{
		{"zero pads to width", 0, " 0"},
		{"single digit pads", 5, " 5"},
		{"rounds down", 9.4, " 9"},
		{"rounds up across digit boundary", 9.6, "10"},
		{"two digits unchanged", 42, "42"},
		{"max two-digit value", 99, "99"},
		{"caps full at 99", 100, "99"},
		{"rounding toward 100 caps at 99", 99.6, "99"},
		{"negative clamps to zero", -3, " 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatPercent(tc.in)
			if got != tc.want {
				t.Fatalf("formatPercent(%v) = %q, want %q", tc.in, got, tc.want)
			}
			if len(got) != percentWidth {
				t.Fatalf("formatPercent(%v) width = %d, want %d", tc.in, len(got), percentWidth)
			}
		})
	}
}
