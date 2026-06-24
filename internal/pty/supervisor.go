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
	"io"
	"os/exec"

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
	ptmx io.ReadWriteCloser // master side; set by the OS-specific start()
	area reserved.Area
}

// New prepares a Supervisor that will run argv (argv[0] is the program).
func New(argv []string, area reserved.Area) *Supervisor {
	return &Supervisor{
		cmd:  exec.Command(argv[0], argv[1:]...), //nolint:gosec // user-chosen shell
		area: area,
	}
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

// Wait blocks until the child exits and returns its exit code (spec §8.2).
// TODO scaffold (plan 04): translate *exec.ExitError into the exit code.
func (s *Supervisor) Wait() (int, error) { return 0, nil }

// TerminateGroup signals the whole child process group on controlled shutdown
// (wrapper SIGTERM/SIGHUP), then Wait reaps it (spec §8.2, §15).
//
// TODO scaffold (plan 05): syscall.Kill(-pgid, SIGTERM/SIGHUP) with a wait
// timeout; escalate if needed. Cmd is started with Setsid + Setctty so the child
// leads its own session/group.
func (s *Supervisor) TerminateGroup(_ string) error { return nil }
