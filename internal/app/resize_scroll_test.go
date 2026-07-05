package app

import (
	"bytes"
	"testing"

	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// The resize path pins the cursor to the last child row only for the one
// combination where the terminal has already clamped it into the bar:
// a clamping terminal (Terminal.app) that shrank in rows. Every other case
// must preserve the cursor via SaveCursor/RestoreCursor, or the input line
// jumps to the bottom on resize.
func TestReapplyScrollRegionAfterResize(t *testing.T) {
	size := terminal.Size{Cols: 80, Rows: 30}
	area := reserved.Default() // 1 bar row → child bottom is 29
	preserve := terminal.SaveCursor + terminal.SetScrollRegion(1, 29) + terminal.RestoreCursor
	pin := terminal.SetScrollRegion(1, 29) + terminal.CursorTo(29, 1)

	tests := []struct {
		name           string
		shrank         bool
		clampsOnShrink bool
		want           string
	}{
		{name: "clamping terminal, shrink", shrank: true, clampsOnShrink: true, want: pin},
		{name: "clamping terminal, grow", shrank: false, clampsOnShrink: true, want: preserve},
		{name: "preserving terminal, shrink", shrank: true, clampsOnShrink: false, want: preserve},
		{name: "preserving terminal, grow", shrank: false, clampsOnShrink: false, want: preserve},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			ctrl := terminal.New(nil, &buf)
			reapplyScrollRegionAfterResize(ctrl, size, area, test.shrank, test.clampsOnShrink)
			if got := buf.String(); got != test.want {
				t.Errorf("wrote %q, want %q", got, test.want)
			}
		})
	}
}
