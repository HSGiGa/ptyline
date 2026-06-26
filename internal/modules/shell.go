package modules

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// Shell renders the basename of the child command ptyline starts.
type Shell struct {
	value string
}

// NewShell creates a shell module from the resolved child argv.
func NewShell(argv []string) *Shell {
	return &Shell{value: shellLabel(argv)}
}

func (m *Shell) ID() status.ModuleID     { return "shell" }
func (m *Shell) Interval() time.Duration { return 0 }

func (m *Shell) Refresh(_ context.Context) status.ModuleSnapshot {
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(m.value),
		UpdatedAt: time.Now(),
	}
}

func shellLabel(argv []string) string {
	if len(argv) == 0 || argv[0] == "" {
		return ""
	}
	name := filepath.Base(argv[0])
	if name == "." || name == string(filepath.Separator) {
		return argv[0]
	}
	return strings.TrimPrefix(name, "-")
}
