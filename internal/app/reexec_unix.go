//go:build unix

package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"syscall"

	"github.com/hsgiga/ptyline/internal/pty"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// reexecSelf replaces the current process image with the binary at binPath,
// transferring the PTY master fd and child PID via PTYLINE_HANDOFF. On success
// it never returns (syscall.Exec replaced the process). On failure it restores
// the terminal to raw mode and the PTY fd to CLOEXEC so the caller can continue
// running normally and fall back to a plain config reload.
//
// Must be called from the event-loop goroutine so no concurrent write to stdout
// is in progress when exec() fires.
func reexecSelf(binPath string, ctrl *terminal.Controller, sup *pty.Supervisor, nonce string, childArgv []string) error {
	fd := sup.MasterFD()
	if fd < 0 {
		return fmt.Errorf("PTY master fd not available")
	}

	// Clear FD_CLOEXEC so the new process image inherits the PTY master.
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_SETFD, 0); errno != 0 {
		return fmt.Errorf("fcntl clear cloexec: %w", errno)
	}

	hs := handoffState{
		Version:   handoffVersion,
		PtyFD:     fd,
		ChildPID:  sup.Pid(),
		Nonce:     nonce,
		ChildArgv: childArgv,
	}
	data, err := json.Marshal(hs)
	if err != nil {
		syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_SETFD, syscall.FD_CLOEXEC) //nolint:errcheck
		return fmt.Errorf("marshal handoff: %w", err)
	}

	env := append(os.Environ(), handoffEnvKey+"="+base64.StdEncoding.EncodeToString(data))

	// Return the terminal to cooked mode without touching the screen. The new
	// process image will call ctrl.Enter() immediately and re-enable raw mode.
	if err := ctrl.SuspendRaw(); err != nil {
		syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_SETFD, syscall.FD_CLOEXEC) //nolint:errcheck
		return fmt.Errorf("suspend raw: %w", err)
	}

	// Replace this process image. On success this never returns.
	if err := syscall.Exec(binPath, os.Args, env); err != nil {
		_ = ctrl.ResumeRaw()
		syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_SETFD, syscall.FD_CLOEXEC) //nolint:errcheck
		return fmt.Errorf("exec %s: %w", binPath, err)
	}
	return nil // unreachable
}

// parseHandoff reads and immediately clears PTYLINE_HANDOFF from the
// environment. Returns:
//   - (state, true, nil)   — valid handoff present
//   - (nil, false, nil)    — no handoff (normal startup)
//   - (nil, true, err)     — handoff present but invalid; caller must exit
func parseHandoff() (*handoffState, bool, error) {
	val := os.Getenv(handoffEnvKey)
	_ = os.Unsetenv(handoffEnvKey)
	if val == "" {
		return nil, false, nil
	}
	data, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, true, fmt.Errorf("incompatible handoff from previous binary; please restart the terminal")
	}
	var hs handoffState
	if err := json.Unmarshal(data, &hs); err != nil || hs.Version != handoffVersion {
		return nil, true, fmt.Errorf("incompatible handoff from previous binary; please restart the terminal")
	}
	return &hs, true, nil
}
