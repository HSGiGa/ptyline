//go:build unix

package pty

// start launches the child inside a Unix PTY at the given size, giving it its own
// session and controlling terminal so shell job control works (spec §8.2).
//
//	TODO scaffold (plan 04): set s.cmd.SysProcAttr = &syscall.SysProcAttr{
//		Setsid: true, Setctty: true} and start on the pty slave, then:
//
//		ptmx, err := pty.StartWithSize(s.cmd, &pty.Winsize{Cols: size.Cols, Rows: size.Rows})
//		s.ptmx = ptmx
func (s *Supervisor) start(size Size) error {
	_ = size
	return nil
}
