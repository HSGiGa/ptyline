// Package snapshot defines the core data types exchanged between module runners
// and the event loop. It is a leaf package with no ptyline-internal imports so
// that both the event bus and the status engine can import it without a cycle.
package snapshot

import (
	"encoding/json"
	"time"
)

// ModuleID is the stable key for a module, used in templates, config, and the
// Modules map.
type ModuleID string

// ModuleValueKind discriminates the typed module value (spec §24.3).
type ModuleValueKind int

const (
	KindText   ModuleValueKind = iota
	KindNumber                 // numeric value
	KindBool                   // boolean value
	KindStatus                 // leveled status value
	KindJSON                   // arbitrary JSON payload
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

// Text constructs a KindText ModuleValue.
func Text(s string) ModuleValue { return ModuleValue{Kind: KindText, Text: s} }

// Number constructs a KindNumber ModuleValue.
func Number(n float64) ModuleValue { return ModuleValue{Kind: KindNumber, Number: n} }

// Bool constructs a KindBool ModuleValue.
func Bool(b bool) ModuleValue { return ModuleValue{Kind: KindBool, Bool: b} }

// Status constructs a KindStatus ModuleValue.
func Status(l StatusLevel, s string) ModuleValue {
	return ModuleValue{Kind: KindStatus, Status: &StatusValue{Level: l, Text: s}}
}

// TextSpan is a styled text segment within a module value.
type TextSpan struct {
	Text  string
	Role  string
	Level StatusLevel
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
