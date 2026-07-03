//go:build unix

package pty

import (
	"os"
	"syscall"

	"github.com/hsgiga/ptyline/internal/reserved"
)

// Adopt builds a Supervisor over an existing PTY master fd and a live child
// process from a re-exec handoff. exec() preserves parent-child relationships,
// so the current process is still the parent of childPID and can wait4() it.
func Adopt(fd, childPID int, area reserved.Area) *Supervisor {
	// The old process image cleared FD_CLOEXEC so exec() kept this fd open.
	// Re-set it now so the fd does not leak into exec-module subprocesses.
	syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_SETFD, syscall.FD_CLOEXEC) //nolint:errcheck
	return &Supervisor{
		ptmx:       os.NewFile(uintptr(fd), "ptyline-adopted-ptmx"),
		adoptedPID: childPID,
		area:       area,
		waitDone:   make(chan struct{}),
	}
}

// waitAdopted reaps the adopted child using wait4. Called from Wait() when
// adoptedPID != 0. exec() does not change parent-child relationships, so this
// process is still the natural parent and wait4 succeeds.
func (s *Supervisor) waitAdopted() (int, error) {
	var ws syscall.WaitStatus
	for {
		_, err := syscall.Wait4(s.adoptedPID, &ws, 0, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return 1, err
		}
		break
	}
	if ws.Signaled() {
		return 128 + int(ws.Signal()), nil
	}
	return ws.ExitStatus(), nil
}
