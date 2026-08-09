package app

import (
	"context"
	"fmt"
	"time"

	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// cursorQueryTimeout bounds how long reapplyScrollRegionAfterShrink waits for
// a DSR cursor-position reply before falling back to the clampsOnShrink
// heuristic. Generous enough to cover a real SSH round trip: the cost of
// waiting is just a bit more time with the cursor already hidden during an
// already-debounced resize (see ResizeRequest/ResizeCommit in app.go).
// A var, not a const, so tests can shrink it instead of sleeping for real.
var cursorQueryTimeout = 300 * time.Millisecond

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

// reapplyScrollRegionAfterShrink is reapplyScrollRegionAfterResize's shrink
// path, upgraded with ground truth: rather than trusting SaveCursor/
// RestoreCursor to have preserved the cursor (a real terminal has to clamp a
// cursor that no longer fits on screen, and it does so BEFORE ptyline reacts
// to the resulting SIGWINCH — so RestoreCursor can only faithfully restore an
// already-clamped, already-wrong position), it queries the terminal's actual
// current cursor row via DSR and pins only if that row is genuinely inside
// the reserved bar. If the terminal never replies in time (or the context is
// canceled), it falls back to the existing clampsOnShrink heuristic exactly
// as reapplyScrollRegionAfterResize would — so behavior can only improve,
// never regress, versus a terminal this fix doesn't (yet) confirm clamps.
func reapplyScrollRegionAfterShrink(ctx context.Context, ctrl *terminal.Controller, posQuery *terminal.PositionQuery,
	size terminal.Size, area reserved.Area, clampsOnShrink bool, trace func(tag, detail string)) {
	ch := posQuery.Arm()
	_, _ = ctrl.Write([]byte(terminal.QueryCursorPosition))
	trace("cursor-query-sent", fmt.Sprintf("size=%dx%d", size.Cols, size.Rows))
	select {
	case pos := <-ch:
		trace("cursor-query-reply", fmt.Sprintf("row=%d col=%d", pos.Row, pos.Col))
		// Ground truth is available either way: act on it directly rather than
		// falling through to the heuristic, which could still pin an in-bounds
		// cursor this exact reply just proved was never clamped.
		if pos.Row > area.ChildRows(size.Rows) {
			ctrl.ApplyScrollRegionAtChildBottom(size, area)
		} else {
			ctrl.ApplyScrollRegion(size, area)
		}
		return
	case <-time.After(cursorQueryTimeout):
		trace("cursor-query-timeout", "")
	case <-ctx.Done():
		trace("cursor-query-timeout", "context canceled")
	}
	reapplyScrollRegionAfterResize(ctrl, size, area, true, clampsOnShrink)
}
