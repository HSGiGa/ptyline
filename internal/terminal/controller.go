package terminal

import (
	"io"
	"os"
)

// Controller manages the real terminal lifecycle. The cardinal rule: whatever it
// changes, it must restore — on normal exit, signal, child exit, or init failure
// after state was modified (spec §8.1, §15, docs/terminal-safety.md).
type Controller struct {
	tty *os.File // controlling terminal (typically os.Stdin/os.Stdout)
	out io.Writer
	raw rawState

	scrollRegionSet bool
	cursorHidden    bool
}

// New creates a Controller over the given tty and output writer.
func New(tty *os.File, out io.Writer) *Controller {
	return &Controller{tty: tty, out: out}
}

// Enter saves terminal state and enables raw mode. On any failure it restores
// whatever was already changed before returning the error.
func (c *Controller) Enter() error {
	if err := c.enableRaw(); err != nil {
		_ = c.Restore()
		return err
	}
	return nil
}

// Restore reverses every modification in the required cleanup order: reset scroll
// region, reset attributes, restore cursor, restore terminal mode, show cursor
// (spec §8.1). Idempotent and safe to call from a signal handler.
func (c *Controller) Restore() error {
	c.ResetScrollRegion()
	c.write(ResetAttrs)
	c.write(RestoreCursor)
	err := c.disableRaw()
	c.write(ShowCursor)
	return err
}

// write emits a control sequence to the real terminal.
// TODO scaffold (plan 03): buffer and handle short writes / EINTR.
func (c *Controller) write(s string) {
	if c.out == nil {
		return
	}
	_, _ = io.WriteString(c.out, s)
}
