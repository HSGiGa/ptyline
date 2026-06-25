// External test package so it can import proxy (which imports shellintegration)
// without a cycle. It proves the Go side is shell-agnostic: one canonical OSC 777
// stream decodes to one ShellState, shared across all shells.
package shellintegration_test

import (
	"testing"

	"github.com/hsgiga/ptyline/internal/proxy"
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/shellintegration"
	"github.com/hsgiga/ptyline/internal/status"
)

// osc frames a key=value pair as the canonical OSC 777 message every template
// emits (ESC ] 777 ; key=value ESC \).
func osc(key, value string) string {
	return "\x1b]777;" + key + "=" + value + "\x1b\\"
}

// The canonical metadata each shell normalizes to, fed through the single filter,
// yields one expected ShellState — no per-shell decode path exists.
func TestCanonicalOSCRoundTrip(t *testing.T) {
	state := status.NewState()
	filter := proxy.NewAnsiFilter(reserved.Default(), state.ApplyShellMeta)

	stream := osc(shellintegration.KeyCWD, "/home/u/project") +
		osc(shellintegration.KeyExitCode, "0") +
		osc(shellintegration.KeyCommand, "go test ./...") +
		osc(shellintegration.KeyDurationMS, "1234") +
		osc(shellintegration.KeyCommand, "")

	if out := filter.Filter([]byte(stream)); len(out) != 0 {
		t.Fatalf("OSC 777 leaked to terminal: %q", out)
	}

	got := state.Shell
	if got.CWD != "/home/u/project" || got.LastExitCode != 0 ||
		got.ActiveCommand != "" || got.LastCommand != "go test ./..." || got.LastDurationMS != 1234 {
		t.Fatalf("ShellState = %+v, want cwd/exit/command/duration populated", got)
	}
}

// Non-whitelisted keys never reach ShellState and are not forwarded.
func TestNonWhitelistedKeyIgnored(t *testing.T) {
	state := status.NewState()
	filter := proxy.NewAnsiFilter(reserved.Default(), state.ApplyShellMeta)
	if out := filter.Filter([]byte(osc("evil_key", "rm -rf /"))); len(out) != 0 {
		t.Fatalf("dropped OSC still forwarded: %q", out)
	}
	if state.Shell != (status.ShellState{}) {
		t.Fatalf("non-whitelisted key mutated state: %+v", state.Shell)
	}
}
