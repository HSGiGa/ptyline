//go:build unix

package modules

import (
	"os/exec"
	"syscall"
)

// setProcessGroup places the command in its own process group and overrides
// cmd.Cancel so a deadline/cancel signals the whole group (negative pgid), not
// just /bin/sh. This reaps children the shell spawned; a grandchild that re-sids
// itself escapes the group, but cmd.WaitDelay still bounds the wait.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid addresses the whole process group (pgid == child pid).
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
