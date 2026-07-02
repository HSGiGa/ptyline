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
	mu     sync.Mutex
	rec    Record
	logger func(tag, msg string) // nil = no-op
}

// New returns an empty diagnostics state.
func New() *State { return &State{} }

// SetLogger registers a sink for all diagnostic events (e.g. PTYLINE_DEBUG file).
// The callback is invoked with a short tag ("ansi", "config", "module") and the
// message. It must be safe to call from multiple goroutines.
func (s *State) SetLogger(fn func(tag, msg string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger = fn
}

func (s *State) log(tag, msg string) {
	if s.logger != nil {
		s.logger(tag, msg)
	}
}

// RecordModuleError stores the most recent module failure.
func (s *State) RecordModuleError(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.LastModuleError = msg
	s.log("module", msg)
}

// RecordConfigWarning stores a config reload or parse warning.
func (s *State) RecordConfigWarning(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.LastConfigWarning = msg
	s.log("config", msg)
}

// RecordAnsiWarning stores the most recent ANSI/OSC filter warning.
func (s *State) RecordAnsiWarning(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.LastAnsiWarning = msg
	s.log("ansi", msg)
}

// Snapshot returns a copy of the current diagnostics for read-only display.
func (s *State) Snapshot() Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rec
}
