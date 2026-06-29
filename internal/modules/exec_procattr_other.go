//go:build !unix

package modules

import "os/exec"

// setProcessGroup is a no-op on non-Unix platforms; cmd.WaitDelay still bounds a
// hung command. Process-group semantics are added with the native backends
// (spec §19).
func setProcessGroup(cmd *exec.Cmd) {}
