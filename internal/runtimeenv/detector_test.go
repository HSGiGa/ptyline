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
