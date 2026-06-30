// Package terminal owns the *real* terminal: raw mode, size, scroll region,
// cursor, and — most importantly — guaranteed state restoration on exit or
// signal.
package terminal

import "fmt"

// VT escape sequences used to drive the real terminal. Centralized here so the
// ANSI filter (package proxy) and the renderer agree on byte-for-byte output.
const (
	ESC = "\x1b"
	CSI = ESC + "["

	SaveCursor    = ESC + "7"
	RestoreCursor = ESC + "8"
	HideCursor    = CSI + "?25l"
	ShowCursor    = CSI + "?25h"
	ResetAttrs    = CSI + "0m"
	ClearLine     = CSI + "2K"
	ClearScreen   = CSI + "2J"

	// ResetScrollRegion clears any scroll margins (full screen).
	ResetScrollRegion = CSI + "r"

	// BeginSyncUpdate/EndSyncUpdate bracket a batch of writes so the terminal
	// presents them atomically (DECSET 2026, "synchronized output"). This is what
	// keeps the bar from flickering on redraw/resize, the same technique tmux uses.
	// Terminals that do not implement it ignore the private mode harmlessly.
	BeginSyncUpdate = CSI + "?2026h"
	EndSyncUpdate   = CSI + "?2026l"
)

// CursorTo returns the sequence to move the cursor to a 1-based (row, col).
func CursorTo(row, col uint16) string {
	return fmt.Sprintf("%s%d;%dH", CSI, row, col)
}

// SetScrollRegion returns the DECSTBM sequence for the inclusive 1-based range
// top..bottom (spec §6).
func SetScrollRegion(top, bottom uint16) string {
	return fmt.Sprintf("%s%d;%dr", CSI, top, bottom)
}
