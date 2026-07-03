package terminal

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// Controller manages the real terminal lifecycle. The cardinal rule: whatever it
// changes, it must restore — on normal exit, signal, child exit, or init failure
// after state was modified.
type Controller struct {
	tty *os.File // controlling terminal (typically os.Stdin/os.Stdout)
	out io.Writer
	raw rawState
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

// write emits a control sequence to the real terminal, draining short writes and
// retrying on EINTR. Errors are intentionally swallowed: control output is
// best-effort (notably during signal-driven cleanup), and a broken pipe means the
// terminal is gone anyway (spec §15).
func (c *Controller) write(s string) {
	if c.out == nil || s == "" {
		return
	}
	b := []byte(s)
	for len(b) > 0 {
		n, err := c.out.Write(b)
		b = b[n:]
		if err == nil {
			continue
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		return
	}
}

// SuspendRaw returns the terminal to cooked mode without touching the screen,
// cursor, or scroll region. Used before syscall.Exec so the new process image
// inherits a clean terminal state without any visual disruption.
func (c *Controller) SuspendRaw() error { return c.disableRaw() }

// ResumeRaw re-enables raw mode after a failed exec attempt so the current
// process image can continue running normally.
func (c *Controller) ResumeRaw() error { return c.enableRaw() }

// Write satisfies io.Writer so the Controller can be the single sink shared with
// the serialized writer; it drains short writes and retries on EINTR.
func (c *Controller) Write(p []byte) (int, error) {
	if c.out == nil {
		return len(p), nil
	}
	total := 0
	for total < len(p) {
		n, err := c.out.Write(p[total:])
		total += n
		if err == nil {
			continue
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		return total, err
	}
	return total, nil
}

var _ io.Writer = (*Controller)(nil)
