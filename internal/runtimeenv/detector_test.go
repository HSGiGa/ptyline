package runtimeenv

import (
	"testing"

	"github.com/hsgiga/ptyline/internal/platform"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name  string
		input platform.Verdict
		want  Kind
	}{
		{name: "Linux", input: platform.Verdict{IsLinux: true}, want: NativeLinux},
		{name: "WSL", input: platform.Verdict{IsLinux: true, IsWSL: true}, want: WSL2},
		{name: "macOS", input: platform.Verdict{IsMacOS: true}, want: MacOS},
		{name: "Windows", input: platform.Verdict{IsWindows: true}, want: NativeWindows},
		{name: "unknown", input: platform.Verdict{}, want: Unknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classify(test.input); got != test.want {
				t.Errorf("classify(%+v) = %s, want %s", test.input, got, test.want)
			}
		})
	}
}

// Only Terminal.app clamps the cursor into the reserved bar row on shrink;
// iTerm2 and the rest preserve it, so they must not get pin-to-bottom.
func TestDetectClampsCursorOnShrink(t *testing.T) {
	tests := []struct {
		name string
		prog string
		set  bool
		want bool
	}{
		{name: "Terminal.app", prog: "Apple_Terminal", set: true, want: true},
		{name: "iTerm2", prog: "iTerm.app", set: true, want: false},
		{name: "WezTerm", prog: "WezTerm", set: true, want: false},
		{name: "ghostty", prog: "ghostty", set: true, want: false},
		{name: "unset", set: false, want: false},
		{name: "empty", prog: "", set: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookup := func(key string) (string, bool) {
				if key == "TERM_PROGRAM" && test.set {
					return test.prog, true
				}
				return "", false
			}
			if got := detectClampsCursorOnShrink(lookup); got != test.want {
				t.Errorf("detectClampsCursorOnShrink(TERM_PROGRAM=%q) = %v, want %v", test.prog, got, test.want)
			}
		})
	}
}

func TestCapabilitiesFor(t *testing.T) {
	tests := []struct {
		kind Kind
		want Capabilities
	}{
		{kind: NativeLinux, want: Capabilities{UnixPTY: true, VTSequences: true, LinuxProcfs: true, LinuxSysfs: true}},
		{kind: WSL2, want: Capabilities{UnixPTY: true, VTSequences: true, LinuxProcfs: true, LinuxSysfs: true, WindowsInterop: true}},
		{kind: MacOS, want: Capabilities{UnixPTY: true, VTSequences: true}},
		{kind: NativeWindows, want: Capabilities{WindowsConPTY: true, VTSequences: true}},
		{kind: Unknown, want: Capabilities{VTSequences: true}},
	}

	for _, test := range tests {
		t.Run(test.kind.String(), func(t *testing.T) {
			if got := capabilitiesFor(test.kind); got != test.want {
				t.Errorf("capabilitiesFor(%s) = %+v, want %+v", test.kind, got, test.want)
			}
		})
	}
}
