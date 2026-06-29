//go:build darwin

package modules

import (
	"context"
	"testing"
)

// These smoke tests exercise the native macOS samplers against the real host.
// They assert availability and plausible ranges rather than exact values.

func TestDarwinCPUProvider(t *testing.T) {
	p := newCPUProvider()
	ctx := context.Background()
	if err := p.Probe(ctx); err != nil {
		t.Fatalf("cpu Probe() error = %v", err)
	}
	s, err := p.Sample(ctx)
	if err != nil {
		t.Fatalf("cpu Sample() error = %v", err)
	}
	if s.Percent < 0 || s.Percent > 100 {
		t.Fatalf("cpu Percent = %v, want [0,100]", s.Percent)
	}
}

func TestDarwinMemoryProvider(t *testing.T) {
	p := newMemoryProvider()
	ctx := context.Background()
	if err := p.Probe(ctx); err != nil {
		t.Fatalf("memory Probe() error = %v", err)
	}
	s, err := p.Sample(ctx)
	if err != nil {
		t.Fatalf("memory Sample() error = %v", err)
	}
	if s.Total == 0 {
		t.Fatal("memory Total = 0, want > 0")
	}
	if s.Used > s.Total {
		t.Fatalf("memory Used = %d > Total = %d", s.Used, s.Total)
	}
	if s.Percent < 0 || s.Percent > 100 {
		t.Fatalf("memory Percent = %v, want [0,100]", s.Percent)
	}
}

func TestDarwinLoadProvider(t *testing.T) {
	p := newLoadProvider()
	ctx := context.Background()
	if err := p.Probe(ctx); err != nil {
		t.Fatalf("load Probe() error = %v", err)
	}
	s, err := p.Sample(ctx)
	if err != nil {
		t.Fatalf("load Sample() error = %v", err)
	}
	if s.Load1 < 0 || s.Load5 < 0 || s.Load15 < 0 {
		t.Fatalf("load = %+v, want non-negative", s)
	}
}

func TestDarwinBatteryProvider(t *testing.T) {
	p := newBatteryProvider()
	ctx := context.Background()
	// A battery may be absent (desktop Mac / CI). Either it probes available with a
	// sane reading, or it reports unavailable — both are correct; a panic or an
	// out-of-range value is not.
	if err := p.Probe(ctx); err != nil {
		t.Skipf("battery unavailable on this host: %v", err)
	}
	s, err := p.Sample(ctx)
	if err != nil {
		t.Fatalf("battery Sample() error = %v", err)
	}
	if s.Percent < 0 || s.Percent > 100 {
		t.Fatalf("battery Percent = %d, want [0,100]", s.Percent)
	}
	if s.State == "" {
		t.Fatal("battery State is empty")
	}
}
