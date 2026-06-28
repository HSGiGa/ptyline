// Package diagnostics holds internal health/debug state surfaced by future
// `ptyline doctor` / `ptyline debug-state` commands and optionally by a warning
// indicator on the bar. See ARCHITECTURE.md §19.
package diagnostics

import (
	"sync"
	"time"
)

// Record is the plain-data diagnostics snapshot (no lock, safe to copy).
type Record struct {
	LastModuleError   string
	LastRenderTime    time.Duration
	LastPtyReadError  string
	LastConfigWarning string
	LastAnsiWarning   string
}

// State wraps a Record with a mutex for concurrent updates.
type State struct {
	mu  sync.Mutex
	rec Record
}

// New returns an empty diagnostics state.
func New() *State { return &State{} }

// RecordModuleError stores the most recent module failure.
func (s *State) RecordModuleError(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.LastModuleError = msg
}

// Snapshot returns a copy of the current diagnostics for read-only display.
// TODO scaffold: expand fields as subsystems report into diagnostics.
func (s *State) Snapshot() Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rec
}
