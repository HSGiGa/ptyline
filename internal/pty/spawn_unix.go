//go:build unix

package pty

import (
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// start launches the child inside a Unix PTY at the given size, giving it its own
// session and controlling terminal so shell job control works (spec §8.2).
func (s *Supervisor) start(size Size) error {
	if s.cmd.SysProcAttr == nil {
		s.cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Setsid: the child leads a new session; Setctty: the PTY slave becomes its
	// controlling terminal, so the kernel routes Ctrl-C/Ctrl-Z to the child's
	// foreground process group (spec §8.2).
	s.cmd.SysProcAttr.Setsid = true
	s.cmd.SysProcAttr.Setctty = true

	ptmx, err := pty.StartWithSize(s.cmd, &pty.Winsize{
		Cols: size.Cols,
		Rows: size.Rows,
	})
	if err != nil {
		return err
	}
	s.ptmx = ptmx
	return nil
}

// setsize applies a new winsize to the master PTY (spec §12).
func (s *Supervisor) setsize(size Size) error {
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: size.Cols, Rows: size.Rows})
}

// terminateGroup signals the child's process group, waits briefly, then escalates
// to SIGKILL if it has not exited (spec §8.2, §15). The reaping Wait runs in the
// loop's exit path.
func (s *Supervisor) terminateGroup(sig string) error {
	pgid := s.Pid() // session leader: pgid == pid; works in both normal and adopted mode
	if pgid == 0 {
		return nil
	}
	signal := syscall.SIGTERM
	if sig == "SIGHUP" {
		signal = syscall.SIGHUP
	}
	// Negative pid addresses the whole process group.
	_ = syscall.Kill(-pgid, signal)

	// Reaping happens in the loop's Wait() goroutine; we only wait for that to
	// observe the exit, escalating to SIGKILL if the group lingers (spec §15).
	select {
	case <-s.waitDone:
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	return nil
}

// exitCode extracts a conventional exit code from an *exec.ExitError, mapping a
// signal-terminated child to 128+signo (spec §8.2).
func exitCode(e *exec.ExitError) int {
	if ws, ok := e.Sys().(syscall.WaitStatus); ok {
		if ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ws.ExitStatus()
	}
	return e.ExitCode()
}
