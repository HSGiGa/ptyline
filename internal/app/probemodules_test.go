package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/status"
)

type fakeProbeMod struct {
	id        string
	available bool
}

func (m fakeProbeMod) ID() status.ModuleID     { return status.ModuleID(m.id) }
func (m fakeProbeMod) Interval() time.Duration { return 0 } // no ticker goroutine in tests

func (m fakeProbeMod) Refresh(context.Context) status.ModuleSnapshot {
	return status.ModuleSnapshot{ID: status.ModuleID(m.id), Value: status.Text("x")}
}

func (m fakeProbeMod) Probe(context.Context) status.ModuleProbe {
	if m.available {
		return status.AvailableProbe()
	}
	return status.UnavailableProbe(errors.New("unavailable"))
}

// TestProbeModManagerReconcile pins the reconcile contract: enabled toggling and
// interval changes take effect on every reload (the original implementation only
// reacted when interval/format changed, so enable/disable was a no-op), and an
// unavailable probe leaves nothing running.
func TestProbeModManagerReconcile(t *testing.T) {
	available := true
	scheduler := status.NewScheduler(func(status.ModuleSnapshot) {})
	spec := probeModSpec{
		id:              "fake",
		defaultInterval: time.Second,
		defaultTimeout:  10 * time.Millisecond,
		build: func(config.ModuleConfig, time.Duration, probeModDeps) status.ProbeModule {
			return fakeProbeMod{id: "fake", available: available}
		},
	}
	mgr := newProbeModManager(context.Background(), scheduler, probeModDeps{}, []probeModSpec{spec})

	cfg := func(enabled bool, intervalMS int) config.Config {
		return config.Config{Modules: map[string]config.ModuleConfig{
			"fake": {Enabled: enabled, IntervalMS: intervalMS},
		}}
	}

	mgr.Reconcile(cfg(true, 1000))
	if e := mgr.entries["fake"]; e == nil || e.interval != time.Second {
		t.Fatalf("after enable: entry=%v, want running with 1s interval", e)
	}

	mgr.Reconcile(cfg(false, 1000))
	if mgr.entries["fake"] != nil {
		t.Fatal("after disable: module should be stopped")
	}

	mgr.Reconcile(cfg(true, 1000))
	if mgr.entries["fake"] == nil {
		t.Fatal("after re-enable: module should run again")
	}

	mgr.Reconcile(cfg(true, 2000))
	if e := mgr.entries["fake"]; e == nil || e.interval != 2*time.Second {
		t.Fatalf("after interval change: entry=%v, want 2s interval", e)
	}

	available = false
	mgr.Reconcile(cfg(true, 3000))
	if mgr.entries["fake"] != nil {
		t.Fatal("unavailable probe: nothing should be scheduled")
	}
}

// TestProbeModManagerOnCWDChange checks that a cwd change resamples only the
// modules that opt in via refreshOnCWD (e.g. {disk}), not every module.
func TestProbeModManagerOnCWDChange(t *testing.T) {
	got := make(chan status.ModuleSnapshot, 8)
	scheduler := status.NewScheduler(func(s status.ModuleSnapshot) { got <- s })
	mk := func(id string, onCWD bool) probeModSpec {
		return probeModSpec{
			id:              id,
			defaultInterval: time.Second,
			defaultTimeout:  10 * time.Millisecond,
			refreshOnCWD:    onCWD,
			build: func(config.ModuleConfig, time.Duration, probeModDeps) status.ProbeModule {
				return fakeProbeMod{id: id, available: true}
			},
		}
	}
	mgr := newProbeModManager(context.Background(), scheduler,
		probeModDeps{}, []probeModSpec{mk("disk", true), mk("load", false)})
	mgr.Reconcile(config.Config{Modules: map[string]config.ModuleConfig{
		"disk": {Enabled: true}, "load": {Enabled: true},
	}})

	// Drain the two initial refreshes (one per started module).
	for i := 0; i < 2; i++ {
		select {
		case <-got:
		case <-time.After(time.Second):
			t.Fatal("missing initial refresh")
		}
	}

	mgr.OnCWDChange()
	select {
	case s := <-got:
		if s.ID != "disk" {
			t.Fatalf("OnCWDChange refreshed %q, want disk", s.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("OnCWDChange did not refresh disk")
	}
	// No other module should have been refreshed.
	select {
	case s := <-got:
		t.Fatalf("OnCWDChange refreshed unexpected module %q", s.ID)
	case <-time.After(50 * time.Millisecond):
	}
}
