package modules

import (
	"context"
	"os"
	"strings"
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

// Refresh is intentionally a no-op: cwd is event-driven, not interval-driven.
// The real value is set by the loop from ShellState.CWD (updated by the OSC
// filter) and tilde-abbreviated via AbbreviateHome; it must never be inferred
// from arbitrary PTY output (spec §13). Interval() == 0 keeps the scheduler from
// ticking this module, so the renderer only ever sees the shell-supplied value.
func (m *CWD) Refresh(_ context.Context) status.ModuleSnapshot {
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(""),
		UpdatedAt: time.Now(),
	}
}

// AbbreviateHome renders a path below home with a leading ~.
func AbbreviateHome(path, home string) string {
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
