//go:build darwin

package app

import (
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// reapplyScrollRegionAfterResize re-establishes the normal-screen scroll region
// after a resize.
//
// On macOS, shrinking the terminal clamps the cursor to the last physical row —
// which is a reserved bar row. SaveCursor/RestoreCursor would faithfully restore
// that clamped position right back into the bar, so input echoes over it until the
// next prompt redraw. Place the cursor at the last child row instead; shells
// reposition their prompt on SIGWINCH regardless.
func reapplyScrollRegionAfterResize(ctrl *terminal.Controller, size terminal.Size, area reserved.Area) {
	ctrl.ApplyScrollRegionAtChildBottom(size, area)
}
