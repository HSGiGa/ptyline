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
