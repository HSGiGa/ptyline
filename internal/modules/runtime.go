package modules

import (
	"context"
	"time"

	"github.com/hsgiga/ptyline/internal/runtimeenv"
	"github.com/hsgiga/ptyline/internal/status"
)

// Runtime renders the normalized runtime profile kind.
type Runtime struct {
	value string
}

// NewRuntime creates a runtime module from the detected runtime profile.
func NewRuntime(profile runtimeenv.Profile) *Runtime {
	return &Runtime{value: profile.Kind.String()}
}

func (m *Runtime) ID() status.ModuleID     { return "runtime" }
func (m *Runtime) Interval() time.Duration { return 0 }

func (m *Runtime) Refresh(_ context.Context) status.ModuleSnapshot {
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(m.value),
		UpdatedAt: time.Now(),
	}
}
