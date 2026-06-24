package status

import (
	"context"
	"testing"
	"time"
)

// blockingModule never returns from Refresh until its context is cancelled, so it
// always overruns the scheduler timeout.
type blockingModule struct{ interval time.Duration }

func (blockingModule) ID() ModuleID              { return "slow" }
func (m blockingModule) Interval() time.Duration { return m.interval }
func (blockingModule) Refresh(ctx context.Context) ModuleSnapshot {
	<-ctx.Done()
	return ModuleSnapshot{ID: "slow", Value: Text("late")}
}

// fastModule returns immediately.
type fastModule struct{ interval time.Duration }

func (fastModule) ID() ModuleID              { return "fast" }
func (m fastModule) Interval() time.Duration { return m.interval }
func (fastModule) Refresh(context.Context) ModuleSnapshot {
	return ModuleSnapshot{ID: "fast", Value: Text("ok")}
}

// A slow module is marked Stale on timeout instead of stalling the bar (plan 07).
func TestRefreshWithTimeoutMarksStale(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	snap := refreshWithTimeout(ctx, blockingModule{})
	if !snap.Stale || snap.Err == nil {
		t.Fatalf("timed-out module not stale: %+v", snap)
	}
	if snap.ID != "slow" {
		t.Fatalf("stale snapshot ID = %q, want slow", snap.ID)
	}
}

func TestRefreshWithTimeoutFastValue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snap := refreshWithTimeout(ctx, fastModule{})
	if snap.Stale || snap.Value.Text != "ok" {
		t.Fatalf("fast module = %+v, want fresh value ok", snap)
	}
}

// Start drives interval modules and emits snapshots; cancelling stops it.
func TestSchedulerStartEmits(t *testing.T) {
	emitted := make(chan ModuleSnapshot, 1)
	s := NewScheduler(func(snap ModuleSnapshot) {
		select {
		case emitted <- snap:
		default:
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx, fastModule{interval: 5 * time.Millisecond}, time.Second)

	select {
	case snap := <-emitted:
		if snap.ID != "fast" {
			t.Fatalf("emitted snapshot ID = %q, want fast", snap.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not emit within 1s")
	}
}

// Event-driven modules (Interval 0) are not ticked by the scheduler.
func TestSchedulerIgnoresEventDriven(t *testing.T) {
	emitted := make(chan ModuleSnapshot, 1)
	s := NewScheduler(func(snap ModuleSnapshot) { emitted <- snap })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx, fastModule{interval: 0}, time.Second)

	select {
	case <-emitted:
		t.Fatal("scheduler ticked an event-driven module")
	case <-time.After(50 * time.Millisecond):
	}
}
