// Package modules implements the built-in status modules. Each satisfies
// status.Module: it refreshes on its own interval (with a timeout for expensive
// ones) and the renderer reads only the cached snapshot (spec §8.7).
package modules

import (
	"context"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// Time renders the current time. MVP module (spec §18).
type Time struct {
	Format   string // strftime-style, e.g. "%H:%M:%S"
	interval time.Duration
}

// NewTime creates a time module.
func NewTime(format string, interval time.Duration) *Time {
	return &Time{Format: format, interval: interval}
}

func (m *Time) ID() status.ModuleID     { return "time" }
func (m *Time) Interval() time.Duration { return m.interval }

// Refresh returns the formatted current time.
// TODO scaffold (plan 08): convert strftime Format to a Go layout (or use a
// strftime helper) and format time.Now().
func (m *Time) Refresh(_ context.Context) status.ModuleSnapshot {
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(time.Now().Format("15:04:05")),
		UpdatedAt: time.Now(),
	}
}
