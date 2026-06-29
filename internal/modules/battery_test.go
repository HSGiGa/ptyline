package modules

import "testing"

func TestParseBatteryCapacity(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "normal", input: "84\n", want: 84},
		{name: "clamp high", input: "101", want: 100},
		{name: "clamp low", input: "-1", want: 0},
		{name: "bad", input: "bad", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBatteryCapacity(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseBatteryCapacity() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBatteryCapacity() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseBatteryCapacity() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNormalizeBatteryState(t *testing.T) {
	if got := normalizeBatteryState("Not charging\n"); got != "not_charging" {
		t.Fatalf("normalizeBatteryState() = %q, want not_charging", got)
	}
	if got := normalizeBatteryState("wat"); got != "unknown" {
		t.Fatalf("normalizeBatteryState() = %q, want unknown", got)
	}
}

func TestFormatBattery(t *testing.T) {
	got := formatBattery(BatterySample{Percent: 84, State: "charging"}, "bat {percent}% {state}")
	if got != "bat 84% charging" {
		t.Fatalf("formatBattery() = %q, want %q", got, "bat 84% charging")
	}
}
