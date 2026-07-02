package status

import (
	"context"
	"time"

	"github.com/hsgiga/ptyline/internal/snapshot"
)

// Type aliases let all existing callers continue to use status.ModuleID,
// status.ModuleSnapshot, etc. without change, while the canonical definitions
// live in the leaf package internal/snapshot.
type (
	ModuleID        = snapshot.ModuleID
	ModuleValueKind = snapshot.ModuleValueKind
	StatusLevel     = snapshot.StatusLevel
	StatusValue     = snapshot.StatusValue
	ModuleValue     = snapshot.ModuleValue
	TextSpan        = snapshot.TextSpan
	ModuleSnapshot  = snapshot.ModuleSnapshot
)

// Re-export ModuleValueKind constants.
const (
	KindText   = snapshot.KindText
	KindNumber = snapshot.KindNumber
	KindBool   = snapshot.KindBool
	KindStatus = snapshot.KindStatus
	KindJSON   = snapshot.KindJSON
)

// Re-export StatusLevel constants.
const (
	LevelOK    = snapshot.LevelOK
	LevelWarn  = snapshot.LevelWarn
	LevelError = snapshot.LevelError
)

// Re-export ModuleValue constructors.
var (
	Text   = snapshot.Text
	Number = snapshot.Number
	Bool   = snapshot.Bool
	Status = snapshot.Status
)

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

// ProbeModule is implemented by modules that need startup discovery before
// scheduling. If Probe reports unavailable, the app should hide the module and
// not start its refresh ticker. Runtime errors after a successful probe are
// represented by stale/error snapshots instead.
type ProbeModule interface {
	Module
	Probe(ctx context.Context) ModuleProbe
}

// ModuleProbe describes whether a module can run in this process on this
// platform/profile.
type ModuleProbe struct {
	Available bool
	Err       error
}

func AvailableProbe() ModuleProbe { return ModuleProbe{Available: true} }

func UnavailableProbe(err error) ModuleProbe { return ModuleProbe{Err: err} }
