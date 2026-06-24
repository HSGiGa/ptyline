package modules

import (
	"context"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// CWD renders the current working directory. The reliable source is shell
// integration (OSC), not the wrapper's own process cwd, since the child shell
// changes directory independently (spec §9). This module therefore mostly
// mirrors ShellState.CWD updated by the OSC filter.
type CWD struct{}

// NewCWD creates a cwd module.
func NewCWD() *CWD { return &CWD{} }

func (m *CWD) ID() status.ModuleID     { return "cwd" }
func (m *CWD) Interval() time.Duration { return 0 }

// Refresh is a no-op placeholder; the value is driven by ShellMeta events. With
// mode = "shell-integration" it shows its fallback (empty) until an adapter
// supplies metadata — it must not infer cwd from arbitrary PTY output (spec §13).
// TODO scaffold (plan 08): read the latest ShellState.CWD and optionally
// abbreviate $HOME to "~".
func (m *CWD) Refresh(_ context.Context) status.ModuleSnapshot {
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(""),
		UpdatedAt: time.Now(),
	}
}
