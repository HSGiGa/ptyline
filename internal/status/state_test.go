package status

import "testing"

func TestApplyShellMeta(t *testing.T) {
	state := NewState()
	state.ApplyShellMeta("cwd", "/work")
	state.ApplyShellMeta("exit_code", "7")
	state.ApplyShellMeta("duration_ms", "42")
	if state.Shell.CWD != "/work" || state.Shell.LastExitCode != 7 || state.Shell.LastDurationMS != 42 {
		t.Fatalf("Shell = %+v", state.Shell)
	}
}
