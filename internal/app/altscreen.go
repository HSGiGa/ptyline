package app

import (
	"github.com/hsgiga/ptyline/internal/proxy"
	"github.com/hsgiga/ptyline/internal/pty"
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// altScreenCoordinator sequences the terminal/PTY operations for alternate-screen
// entry and exit (spec §11). It stays in the app package because it coordinates
// across terminal, pty, and proxy — moving it deeper would create import cycles.
//
// The protocol is two-phase: the ANSI filter calls SetPending when it detects
// a ?1049h/l sequence mid-stream; WriteOutput calls FlushPending once the bytes
// have been written to the terminal so the transition runs on the correct screen.
type altScreenCoordinator struct {
	ctrl   *terminal.Controller
	sup    *pty.Supervisor
	writer *proxy.TerminalWriter
	state  *status.StatusState
	area   *reserved.Area
	redraw func()

	pendingVal bool
	hasPending bool

	// resizedDuringAlt is set when a resize event occurs while the alternate
	// screen is active. The pre-alt cursor position was saved before the resize
	// and may now lie inside the reserved bar rows if the terminal shrank.
	// Cleared on alt entry so repeated alt sessions each track independently.
	resizedDuringAlt bool
}

// SetPending records an incoming alt-screen transition for deferred execution.
func (c *altScreenCoordinator) SetPending(active bool) {
	c.pendingVal = active
	c.hasPending = true
}

// FlushPending executes and clears a pending transition. Returns true if a
// transition was applied. Must be called after child bytes have been written.
func (c *altScreenCoordinator) FlushPending() bool {
	if !c.hasPending {
		return false
	}
	c.Apply(c.pendingVal)
	c.hasPending = false
	return true
}

// MarkResizedDuringAlt records that a terminal resize occurred while the
// alternate screen was active. The ResizeCommit handler calls this so that
// Apply(false) can choose the correct scroll-region strategy on alt exit.
func (c *altScreenCoordinator) MarkResizedDuringAlt() { c.resizedDuringAlt = true }

// Apply executes the alt-screen entry or exit procedure immediately.
func (c *altScreenCoordinator) Apply(active bool) {
	c.state.Terminal.AlternateScreen = active
	if active {
		c.resizedDuringAlt = false // fresh entry: reset resize tracker
		c.writer.OnAltEnter()
		c.ctrl.ResetScrollRegion()
		_ = c.sup.ResizeFull(pty.Size{Cols: c.state.Terminal.Cols, Rows: c.state.Terminal.Rows})
		return
	}
	// Leaving alt: ?1049l has already restored the normal screen and the pre-alt
	// cursor. Two sub-cases:
	//
	//   No resize during alt — the pre-alt cursor was saved while the scroll
	//   region confined the child to rows 1..childBottom, so it is guaranteed
	//   to be inside the child area. Use ApplyScrollRegion (SaveCursor/DECSTBM/
	//   RestoreCursor) to preserve the exact position. Forcing the cursor to the
	//   last child row here breaks shells that briefly probe the alt screen at
	//   startup (e.g. fish's terminal-capability query).
	//
	//   Resize during alt — the saved pre-alt position is based on the old
	//   geometry. If the terminal shrank, that row may now be inside the reserved
	//   bar area. Use ApplyScrollRegionAtChildBottom to guarantee a safe landing.
	size := terminal.Size{Cols: c.state.Terminal.Cols, Rows: c.state.Terminal.Rows}
	_ = c.sup.Resize(pty.Size{Cols: size.Cols, Rows: size.Rows})
	if c.resizedDuringAlt {
		c.resizedDuringAlt = false
		c.ctrl.ApplyScrollRegionAtChildBottom(size, *c.area)
	} else {
		c.ctrl.ApplyScrollRegion(size, *c.area)
	}
	c.redraw()
}
