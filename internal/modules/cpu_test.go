package modules

import "testing"

func TestParseProcStatCPU(t *testing.T) {
	got, err := parseProcStatCPU("cpu  100 2 30 800 10 0 5 0 0 0\ncpu0 1 2 3 4\n")
	if err != nil {
		t.Fatalf("parseProcStatCPU() error = %v", err)
	}
	want := cpuTimes{Idle: 810, Total: 947}
	if got != want {
		t.Fatalf("parseProcStatCPU() = %+v, want %+v", got, want)
	}
}

func TestParseProcStatCPUErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "missing aggregate", input: "cpu0 1 2 3 4\n"},
		{name: "bad value", input: "cpu 1 x 3 4\n"},
		{name: "too few fields", input: "cpu 1 2 3\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseProcStatCPU(tt.input); err == nil {
				t.Fatal("parseProcStatCPU() error = nil, want error")
			}
		})
	}
}

func TestCPUPercent(t *testing.T) {
	got := cpuPercent(
		cpuTimes{Idle: 80, Total: 100},
		cpuTimes{Idle: 90, Total: 200},
	)
	if got.Percent != 90 {
		t.Fatalf("cpuPercent() = %.2f, want 90", got.Percent)
	}
}

func TestFormatCPU(t *testing.T) {
	got := formatCPU(CPUSample{Percent: 12.4}, "cpu {percent}%")
	if got != "cpu 12%" {
		t.Fatalf("formatCPU() = %q, want %q", got, "cpu 12%")
	}
}
