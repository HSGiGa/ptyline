package status

import (
	"context"
	"encoding/json"
	"time"
)

// ModuleID is the stable key for a module, used in templates, config, and the
// Modules map.
type ModuleID string

// Module produces a value for the bar. Expensive modules refresh on their own
// interval with a timeout; the renderer always reads the cached snapshot, never
// triggering work per redraw (spec §8.7).
type Module interface {
	ID() ModuleID
	// Interval is how often Refresh should run. Zero means event-driven only.
	Interval() time.Duration
	// Refresh computes a fresh value, honoring ctx's deadline (the timeout).
	Refresh(ctx context.Context) ModuleSnapshot
}

// ModuleValueKind discriminates the typed module value (spec §24.3).
type ModuleValueKind int

const (
	KindText ModuleValueKind = iota
	KindNumber
	KindBool
	KindStatus
	KindJSON
)

// StatusLevel is the semantic level of a StatusValue; it maps to a theme token,
// not a raw color (spec §24.4, ARCHITECTURE.md §16).
type StatusLevel string

const (
	LevelOK    StatusLevel = "ok"
	LevelWarn  StatusLevel = "warn"
	LevelError StatusLevel = "error"
)

// StatusValue is a leveled text value.
type StatusValue struct {
	Level StatusLevel
	Text  string
}

// ModuleValue is a typed module result, not a bare string, so the renderer can
// format numbers, show errors, and handle staleness (spec §24.3). The active
// field is selected by Kind.
type ModuleValue struct {
	Kind   ModuleValueKind
	Text   string
	Number float64
	Bool   bool
	Status *StatusValue
	JSON   json.RawMessage
}

type TextSpan struct {
	Text  string
	Role  string
	Level StatusLevel
}

// Constructors for the common kinds.
func Text(s string) ModuleValue    { return ModuleValue{Kind: KindText, Text: s} }
func Number(n float64) ModuleValue { return ModuleValue{Kind: KindNumber, Number: n} }
func Bool(b bool) ModuleValue      { return ModuleValue{Kind: KindBool, Bool: b} }
func Status(l StatusLevel, s string) ModuleValue {
	return ModuleValue{Kind: KindStatus, Status: &StatusValue{Level: l, Text: s}}
}

// ModuleSnapshot is one cached module result with freshness/error metadata
// (spec §24.3). The renderer can show stale data dimmed and hide errored modules.
type ModuleSnapshot struct {
	ID        ModuleID
	Value     ModuleValue
	UpdatedAt time.Time
	Stale     bool
	Err       error
	Spans     []TextSpan
	// AnimationSuppressed, when true, stops the renderer from applying the
	// configured animation even if one is set. Default false = animate normally.
	// Set by modules that control their own animation timing (e.g. command
	// suppresses when no command is running).
	AnimationSuppressed bool
}
