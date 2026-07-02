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

// Apply executes the alt-screen entry or exit procedure immediately.
func (c *altScreenCoordinator) Apply(active bool) {
	c.state.Terminal.AlternateScreen = active
	if active {
		c.writer.OnAltEnter()
		c.ctrl.ResetScrollRegion()
		_ = c.sup.ResizeFull(pty.Size{Cols: c.state.Terminal.Cols, Rows: c.state.Terminal.Rows})
		return
	}
	// Leaving alt: the ?1049l has already restored the normal screen and the
	// pre-alt cursor. Because the normal-screen scroll region confined the child
	// to rows 1..childBottom, that restored cursor is always inside the child
	// region (never on the reserved bar row), so we only need to re-establish the
	// scroll region — with save/restore so DECSTBM's homing side effect doesn't
	// move the cursor. Pinning it to the last child row here (as an earlier fix
	// did) is wrong for shells that merely probe the alt screen at startup, e.g.
	// fish's terminal-capability query: it forced their first prompt to the bottom.
	_ = c.sup.Resize(pty.Size{Cols: c.state.Terminal.Cols, Rows: c.state.Terminal.Rows})
	c.ctrl.ApplyScrollRegion(terminal.Size{Cols: c.state.Terminal.Cols, Rows: c.state.Terminal.Rows}, *c.area)
	c.redraw()
}
