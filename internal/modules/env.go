package modules

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// Env renders configured environment variables. It is event-driven from shell
// integration; Refresh provides only the initial parent-environment snapshot.
type Env struct {
	names []string
}

// NewEnv creates an env module for one or more environment variable names.
func NewEnv(names []string) *Env {
	return &Env{names: append([]string(nil), names...)}
}

func (m *Env) ID() status.ModuleID     { return "env" }
func (m *Env) Interval() time.Duration { return 0 }

func (m *Env) Refresh(_ context.Context) status.ModuleSnapshot {
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(formatEnvValues(m.names, os.Getenv)),
		UpdatedAt: time.Now(),
	}
}

func envValue(name string) string {
	if name == "" {
		return ""
	}
	return os.Getenv(name)
}

func formatEnvValues(names []string, lookup func(string) string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return lookup(names[0])
	}
	values := make([]string, 0, len(names))
	for _, name := range names {
		value := lookup(name)
		if value == "" {
			continue
		}
		values = append(values, name+"="+value)
	}
	return strings.Join(values, " ")
}
