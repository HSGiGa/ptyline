package modules

import (
	"context"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// Git renders the current branch (and later dirty state). It is the canonical
// "expensive module": it must run on its own interval with a timeout, and the
// renderer reads only the cached snapshot — never shelling out per redraw
// (spec §8.7). This is post-core (plan 15), included to exercise the caching
// abstraction.
type Git struct {
	interval time.Duration
	timeout  time.Duration
}

// NewGit creates a git module with refresh interval and per-refresh timeout.
func NewGit(interval, timeout time.Duration) *Git {
	return &Git{interval: interval, timeout: timeout}
}

func (m *Git) ID() status.ModuleID     { return "git" }
func (m *Git) Interval() time.Duration { return m.interval }

// Refresh runs `git` under ctx's deadline. On timeout it returns a stale
// snapshot rather than blocking the bar.
// TODO scaffold (plan 15): exec `git rev-parse --abbrev-ref HEAD` (+ status for
// dirty) with the timeout; mark Stale/Err appropriately.
func (m *Git) Refresh(ctx context.Context) status.ModuleSnapshot {
	_ = ctx
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(""),
		UpdatedAt: time.Now(),
	}
}
