//go:build !darwin

package app

import (
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// reapplyScrollRegionAfterResize re-establishes the normal-screen scroll region
// after a resize while preserving the user's cursor position.
//
// On Linux/WSL the terminal does not clamp the cursor into the physical last row
// on shrink (the scroll region protects the reserved bar rows), so SaveCursor →
// DECSTBM → RestoreCursor keeps the cursor exactly where it was. Forcing it to the
// last child row here is what made text "jump" to the bottom on every resize/split.
func reapplyScrollRegionAfterResize(ctrl *terminal.Controller, size terminal.Size, area reserved.Area) {
	ctrl.ApplyScrollRegion(size, area)
}
