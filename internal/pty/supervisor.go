// Package pty creates and manages the child pseudo-terminal: it spawns the shell,
// keeps the child sized to rows-minus-reserved, monitors its lifecycle, and
// surfaces the exit code that ptyline itself exits with (spec §8.2).
//
// On Unix the supervisor establishes a new session and controlling terminal for
// the child, makes the child's process group the foreground group of the PTY, and
// preserves normal shell job control. It owns the child *process group*, not just
// the shell PID. Terminal-generated signals (Ctrl-C/Ctrl-Z) ride through as PTY
// input bytes so the kernel delivers them to the child's foreground group; on
// wrapper shutdown (SIGTERM/SIGHUP) the supervisor terminates the child process
// group, waits for it, then the terminal is restored (best-effort for SIGKILL).
package pty

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/hsgiga/ptyline/internal/reserved"
)

// Size is the child PTY size in cells.
type Size struct {
	Cols uint16
	Rows uint16
}

// Supervisor owns one child PTY and its process.
type Supervisor struct {
	cmd  *exec.Cmd
	ptmx *os.File // master side; set by the OS-specific start()
	area reserved.Area

	waitOnce sync.Once
	waitCode int
	waitErr  error
	waitDone chan struct{} // closed once the child has been reaped
}

// New prepares a Supervisor that will run argv (argv[0] is the program).
func New(argv []string, area reserved.Area) *Supervisor {
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // user-chosen shell
	cmd.Env = os.Environ()
	return &Supervisor{
		cmd:      cmd,
		area:     area,
		waitDone: make(chan struct{}),
	}
}

// SetEnv appends or overrides one environment variable in the child process.
func (s *Supervisor) SetEnv(key, value string) {
	s.cmd.Env = setEnv(s.cmd.Env, key, value)
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// Start spawns the child inside a new PTY sized to terminal rows minus the
// reserved rows. Delegates to the build-tagged start().
func (s *Supervisor) Start(terminal Size) error {
	return s.start(s.childSize(terminal))
}

// childSize applies the reserved-rows rule (spec §8.2).
func (s *Supervisor) childSize(terminal Size) Size {
	return Size{Cols: terminal.Cols, Rows: s.area.ChildRows(terminal.Rows)}
}

// PTY returns the master side for the IO proxy (read child output, write stdin).
func (s *Supervisor) PTY() io.ReadWriteCloser { return s.ptmx }

// Pid returns the child shell's process id (also its process-group id, since it
// is started as a session leader). Zero before Start.
func (s *Supervisor) Pid() int {
	if s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

// Wait blocks until the child exits and returns its exit code (spec §8.2). A
// clean exit is 0; a non-zero status is taken from the *exec.ExitError; a
// signal-killed child yields 128+signo via the OS-specific decoder. It is safe
// to call concurrently and repeatedly: the underlying cmd.Wait runs exactly once
// and the result is cached.
func (s *Supervisor) Wait() (int, error) {
	s.waitOnce.Do(func() {
		err := s.cmd.Wait()
		switch err {
		case nil:
			s.waitCode = 0
		default:
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				s.waitCode = exitCode(exitErr)
			} else {
				s.waitCode, s.waitErr = 1, err
			}
		}
		close(s.waitDone)
	})
	<-s.waitDone
	return s.waitCode, s.waitErr
}

// SetArea updates the reserved area (after a bar-height change from an overlay).
func (s *Supervisor) SetArea(area reserved.Area) { s.area = area }

// Resize updates the child PTY size to terminal rows minus reserved rows. Called
// (debounced) on every real-terminal resize (spec §8.2, §12).
func (s *Supervisor) Resize(terminal Size) error {
	if s.ptmx == nil {
		return nil
	}
	return s.setsize(s.childSize(terminal))
}

// ResizeFull sizes the child to the full terminal height (no reserved rows),
// used while the alternate screen is active and the child owns every row
// (spec §11).
func (s *Supervisor) ResizeFull(terminal Size) error {
	if s.ptmx == nil {
		return nil
	}
	return s.setsize(terminal)
}

// TerminateGroup signals the whole child process group on controlled shutdown
// (wrapper SIGTERM/SIGHUP), then Wait reaps it (spec §8.2, §15). The child leads
// its own session/group (Setsid), so -pid addresses the group.
func (s *Supervisor) TerminateGroup(sig string) error {
	return s.terminateGroup(sig)
}
