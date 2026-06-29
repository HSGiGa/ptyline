package modules

import (
	"testing"
)

func TestParseLoadavg(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    LoadSample
		wantErr bool
	}{
		{
			name:  "linux proc loadavg",
			input: "0.12 0.34 0.56 1/234 5678\n",
			want:  LoadSample{Load1: 0.12, Load5: 0.34, Load15: 0.56},
		},
		{
			name:    "too few fields",
			input:   "0.12 0.34",
			wantErr: true,
		},
		{
			name:    "bad load1",
			input:   "bad 0.34 0.56",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLoadavg(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseLoadavg() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLoadavg() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseLoadavg() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFormatLoad(t *testing.T) {
	sample := LoadSample{Load1: 1.2, Load5: 3.4, Load15: 5.6}
	got := formatLoad(sample, "load {load1}/{load5}/{load15}")
	want := "load 1.20/3.40/5.60"
	if got != want {
		t.Fatalf("formatLoad() = %q, want %q", got, want)
	}
}
