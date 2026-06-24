package modules

import (
	"context"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// Static renders a fixed string. MVP module (spec §18); useful for labels and
// separators in the bar format.
type Static struct {
	id   string
	text string
}

// NewStatic creates a static-text module with the given id and text.
func NewStatic(id, text string) *Static {
	return &Static{id: id, text: text}
}

func (m *Static) ID() status.ModuleID     { return status.ModuleID(m.id) }
func (m *Static) Interval() time.Duration { return 0 }

// Refresh returns the constant text.
func (m *Static) Refresh(_ context.Context) status.ModuleSnapshot {
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(m.text),
		UpdatedAt: time.Now(),
	}
}
