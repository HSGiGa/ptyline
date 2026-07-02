//go:build unix

package app

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func sendReloadSignal() int {
	pidStr := os.Getenv("PTYLINE_PID")
	if pidStr == "" {
		fmt.Fprintln(os.Stderr, "ptyline: --reload requires $PTYLINE_PID (not running inside ptyline)")
		return 1
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ptyline: invalid $PTYLINE_PID %q: %v\n", pidStr, err)
		return 1
	}
	// Verify the target process is actually ptyline before signaling. On Unix,
	// os.FindProcess always succeeds, so we check the running process name.
	if name, err := processName(pid); err == nil && !strings.Contains(name, "ptyline") {
		fmt.Fprintf(os.Stderr, "ptyline: PID %d is not ptyline (got %q)\n", pid, name)
		return 1
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ptyline: process %d not found: %v\n", pid, err)
		return 1
	}
	if err := p.Signal(syscall.SIGUSR1); err != nil {
		fmt.Fprintf(os.Stderr, "ptyline: reload failed: %v\n", err)
		return 1
	}
	return 0
}

// processName returns the executable name of process pid.
// Uses ps(1) which is available on both macOS and Linux.
func processName(pid int) (string, error) {
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
