package modules

import (
	"context"
	"os"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// Env renders the value of a configured environment variable. It is static for
// the process lifetime; an empty name or missing variable hides the block.
type Env struct {
	name  string
	value string
}

// NewEnv creates an env module for one environment variable name.
func NewEnv(name string) *Env {
	return &Env{name: name, value: envValue(name)}
}

func (m *Env) ID() status.ModuleID     { return "env" }
func (m *Env) Interval() time.Duration { return 0 }

func (m *Env) Refresh(_ context.Context) status.ModuleSnapshot {
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(m.value),
		UpdatedAt: time.Now(),
	}
}

func envValue(name string) string {
	if name == "" {
		return ""
	}
	return os.Getenv(name)
}
