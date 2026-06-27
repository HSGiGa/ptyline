//go:build unix

package app

import (
	"fmt"
	"os"
	"strconv"
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
