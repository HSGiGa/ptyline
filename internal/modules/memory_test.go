package modules

import "testing"

func TestParseMeminfo(t *testing.T) {
	input := "MemTotal:       1000 kB\nMemAvailable:    250 kB\nMemFree:         100 kB\n"
	got, err := parseMeminfo(input)
	if err != nil {
		t.Fatalf("parseMeminfo() error = %v", err)
	}
	if got.Total != 1024000 || got.Available != 256000 || got.Used != 768000 {
		t.Fatalf("parseMeminfo() = %+v", got)
	}
	if got.Percent != 75 {
		t.Fatalf("Percent = %.2f, want 75", got.Percent)
	}
}

func TestParseMeminfoErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "missing total", input: "MemAvailable: 1 kB\n"},
		{name: "missing available", input: "MemTotal: 1 kB\n"},
		{name: "bad value", input: "MemTotal: bad kB\nMemAvailable: 1 kB\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseMeminfo(tt.input); err == nil {
				t.Fatal("parseMeminfo() error = nil, want error")
			}
		})
	}
}

func TestFormatMemory(t *testing.T) {
	sample := MemorySample{
		Total:     1024 * 1024 * 10,
		Available: 1024 * 1024 * 3,
		Used:      1024 * 1024 * 7,
		Percent:   70,
	}
	got := formatMemory(sample, "mem {percent}% {used_mb}/{total_mb}MB")
	want := "mem 70% 7/10MB"
	if got != want {
		t.Fatalf("formatMemory() = %q, want %q", got, want)
	}
}
