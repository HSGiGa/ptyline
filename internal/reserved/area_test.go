package reserved

import "testing"

func TestChildRows(t *testing.T) {
	a := Default() // {Bottom, 1}
	cases := []struct {
		terminalRows uint16
		want         uint16
	}{
		{30, 29},
		{2, 1},
		{1, 1}, // never below 1, even when terminal is tiny (spec §15)
		{0, 1},
	}
	for _, c := range cases {
		if got := a.ChildRows(c.terminalRows); got != c.want {
			t.Errorf("ChildRows(%d) = %d, want %d", c.terminalRows, got, c.want)
		}
	}
}

func TestBarTopRow(t *testing.T) {
	a := Default()
	if got := a.BarTopRow(30); got != 30 {
		t.Errorf("BarTopRow(30) = %d, want 30", got)
	}
}

func TestChildRowsMultiLine(t *testing.T) {
	a := Area{Edge: Bottom, Rows: 2}
	if got := a.ChildRows(30); got != 28 {
		t.Errorf("ChildRows(30) with 2 reserved = %d, want 28", got)
	}
}
