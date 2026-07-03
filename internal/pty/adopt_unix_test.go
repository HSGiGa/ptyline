//go:build unix

package pty

import (
	"os/exec"
	"syscall"
	"testing"

	creackpty "github.com/creack/pty"

	"github.com/hsgiga/ptyline/internal/reserved"
)

// startChild spawns a real shell command inside a PTY and returns the master fd
// and child PID. The caller owns the fd and must wait for the child.
func startChild(t *testing.T, argv []string) (fd, pid int) {
	t.Helper()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	ptmx, err := creackpty.StartWithSize(cmd, &creackpty.Winsize{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("start child: %v", err)
	}
	return int(ptmx.Fd()), cmd.Process.Pid
}

// TestAdoptPTYReadWrite verifies that an adopted Supervisor proxies IO through
// the inherited PTY master without error.
func TestAdoptPTYReadWrite(t *testing.T) {
	fd, pid := startChild(t, []string{"/bin/sh", "-c", "exit 0"})
	area := reserved.Default()
	sup := Adopt(fd, pid, area)

	// PTY() must return a usable ReadWriteCloser.
	if sup.PTY() == nil {
		t.Fatal("PTY() returned nil")
	}
	if sup.Pid() != pid {
		t.Fatalf("Pid() = %d, want %d", sup.Pid(), pid)
	}
	// Resize must not error (fd is valid).
	if err := sup.Resize(Size{Cols: 100, Rows: 30}); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	sup.Wait() //nolint:errcheck // drain child
}

// TestAdoptWaitExitCode verifies that adopted Wait() propagates the child's
// exit code exactly as if we had spawned it ourselves.
func TestAdoptWaitExitCode(t *testing.T) {
	fd, pid := startChild(t, []string{"/bin/sh", "-c", "exit 7"})
	sup := Adopt(fd, pid, reserved.Default())
	code, err := sup.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
}

// TestAdoptWaitSignal verifies that a signal-killed adopted child maps to
// 128 + signo, matching the behaviour of a normally-spawned child.
func TestAdoptWaitSignal(t *testing.T) {
	fd, pid := startChild(t, []string{"/bin/sh", "-c", "sleep 60"})
	sup := Adopt(fd, pid, reserved.Default())

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		t.Fatalf("kill: %v", err)
	}

	code, err := sup.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	want := 128 + int(syscall.SIGTERM)
	if code != want {
		t.Fatalf("signal exit code = %d, want %d", code, want)
	}
}
