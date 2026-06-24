package terminal

// rawState holds the saved terminal mode so it can be restored exactly.
//
// TODO scaffold (plan 03): store *term.State from term.MakeRaw(fd) and restore
// with term.Restore(fd, state).
type rawState struct {
	saved bool
	// state *term.State
}

// enableRaw puts the controlling tty into raw mode and records the prior state.
func (c *Controller) enableRaw() error {
	// TODO scaffold: c.raw.state, err = term.MakeRaw(int(c.tty.Fd()))
	c.raw.saved = true
	return nil
}

// disableRaw restores the saved terminal mode. Safe to call multiple times.
func (c *Controller) disableRaw() error {
	if !c.raw.saved {
		return nil
	}
	// TODO scaffold: term.Restore(int(c.tty.Fd()), c.raw.state)
	c.raw.saved = false
	return nil
}
