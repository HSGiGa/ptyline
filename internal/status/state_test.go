package status

import "testing"

func TestApplyShellMeta(t *testing.T) {
	state := NewState()
	state.ApplyShellMeta("cwd", "/work")
	state.ApplyShellMeta("command", "go test ./...")
	state.ApplyShellMeta("exit_code", "7")
	state.ApplyShellMeta("duration_ms", "42")
	if state.Shell.CWD != "/work" || state.Shell.LastExitCode != 7 || state.Shell.LastDurationMS != 42 {
		t.Fatalf("Shell = %+v", state.Shell)
	}
	if state.Shell.ActiveCommand != "go test ./..." || state.Shell.LastCommand != "go test ./..." {
		t.Fatalf("command state = %+v", state.Shell)
	}
	state.ApplyShellMeta("command", "")
	if state.Shell.ActiveCommand != "" || state.Shell.LastCommand != "go test ./..." {
		t.Fatalf("cleared command state = %+v", state.Shell)
	}
}
