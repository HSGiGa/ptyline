package pty

// Resize updates the child PTY size to terminal rows minus reserved rows. Called
// (debounced) on every real-terminal resize (spec §8.2, §12).
//
// TODO scaffold (plan 04): apply via pty.Setsize(ptmx, winsize) on Unix and the
// ConPTY resize API on Windows.
func (s *Supervisor) Resize(terminal Size) error {
	_ = s.childSize(terminal)
	return nil
}
