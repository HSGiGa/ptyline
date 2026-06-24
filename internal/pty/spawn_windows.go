//go:build windows

package pty

// start launches the child via the Windows ConPTY backend.
//
// TODO scaffold (plan 04): create a pseudo console (CreatePseudoConsole) sized to
// `size`, attach the child process, and expose the pipe as s.ptmx.
func (s *Supervisor) start(size Size) error {
	_ = size
	return nil
}
