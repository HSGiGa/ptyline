//go:build windows

package pty

import "os/exec"

// start launches the child via the Windows ConPTY backend.
//
// TODO scaffold (plan 04): create a pseudo console (CreatePseudoConsole) sized to
// `size`, attach the child process, and expose the pipe as s.ptmx. ConPTY is
// post-MVP (spec §19); the Linux/WSL MVP does not exercise this path.
func (s *Supervisor) start(size Size) error {
	_ = size
	return nil
}

// setsize is the ConPTY resize stub (post-MVP).
func (s *Supervisor) setsize(size Size) error {
	_ = size
	return nil
}

// terminateGroup is the ConPTY shutdown stub (post-MVP).
func (s *Supervisor) terminateGroup(sig string) error {
	_ = sig
	return nil
}

// exitCode extracts a conventional exit code from an *exec.ExitError.
func exitCode(e *exec.ExitError) int {
	return e.ExitCode()
}
