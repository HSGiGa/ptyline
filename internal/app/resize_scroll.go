package app

import (
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// reapplyScrollRegionAfterResize re-establishes the normal-screen scroll region
// after a resize.
//
// Most terminals (Linux emulators, iTerm2, WezTerm, kitty) respect the scroll
// region on shrink and leave the cursor where it was, so SaveCursor → DECSTBM →
// RestoreCursor preserves the user's position exactly. Forcing the cursor to the
// last child row on those terminals is what made the input line "jump" to the
// bottom on every resize/split.
//
// Terminal.app is the exception (Capabilities.ClampsCursorOnShrink): shrinking
// the window clamps the cursor to the last physical row — a reserved bar row —
// and SaveCursor/RestoreCursor would faithfully restore that clamped position
// right back into the bar, so input echoes over it until the next prompt redraw.
// There, and only when the terminal actually shrank in rows (grow and width-only
// resizes never clamp), place the cursor at the last child row instead; shells
// reposition their prompt on SIGWINCH regardless.
func reapplyScrollRegionAfterResize(ctrl *terminal.Controller, size terminal.Size, area reserved.Area, shrank, clampsOnShrink bool) {
	if clampsOnShrink && shrank {
		ctrl.ApplyScrollRegionAtChildBottom(size, area)
		return
	}
	ctrl.ApplyScrollRegion(size, area)
}
