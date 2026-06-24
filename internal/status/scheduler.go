package status

import (
	"context"
	"time"
)

// Scheduler runs interval-driven modules off the event loop. Each module with a
// positive Interval gets its own goroutine that calls Refresh under a timeout and
// emits the resulting snapshot; the loop applies it to StatusState. This keeps all
// expensive/blocking work time-bounded and out of the render path (spec §8.7, §16).
type Scheduler struct {
	emit func(ModuleSnapshot)
}

// NewScheduler creates a scheduler that hands finished snapshots to emit. emit is
// expected to enqueue a ModuleUpdated event so only the loop mutates state.
func NewScheduler(emit func(ModuleSnapshot)) *Scheduler {
	return &Scheduler{emit: emit}
}

// Start launches a refresh ticker for m. Event-driven modules (Interval <= 0,
// e.g. cwd/hostname) are ignored — their values arrive via ShellMeta or a one-off
// initial refresh. The goroutine stops when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context, m Module, timeout time.Duration) {
	if m.Interval() <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(m.Interval())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cctx, cancel := context.WithTimeout(ctx, timeout)
				s.emit(refreshWithTimeout(cctx, m))
				cancel()
			}
		}
	}()
}

// refreshWithTimeout runs m.Refresh but never blocks past ctx's deadline. A module
// that overruns yields a stale snapshot carrying ctx.Err(); its late result (if it
// ever arrives) is discarded, and the next tick supersedes it. The renderer shows
// the last good value dimmed rather than stalling the bar.
func refreshWithTimeout(ctx context.Context, m Module) ModuleSnapshot {
	done := make(chan ModuleSnapshot, 1)
	go func() { done <- m.Refresh(ctx) }()
	select {
	case snap := <-done:
		return snap
	case <-ctx.Done():
		return ModuleSnapshot{
			ID:        m.ID(),
			Stale:     true,
			Err:       ctx.Err(),
			UpdatedAt: time.Now(),
		}
	}
}
