package terminal

import "golang.org/x/term"

// rawState holds the saved terminal mode so it can be restored exactly.
type rawState struct {
	saved bool
	state *term.State
}

// enableRaw puts the controlling tty into raw mode and records the prior state.
func (c *Controller) enableRaw() error {
	state, err := term.MakeRaw(int(c.tty.Fd()))
	if err != nil {
		return err
	}
	c.raw.state = state
	c.raw.saved = true
	return nil
}

// disableRaw restores the saved terminal mode. Safe to call multiple times.
func (c *Controller) disableRaw() error {
	if !c.raw.saved {
		return nil
	}
	err := term.Restore(int(c.tty.Fd()), c.raw.state)
	c.raw.saved = false
	c.raw.state = nil
	return err
}
