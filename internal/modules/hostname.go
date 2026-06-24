package modules

import (
	"context"
	"os"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// Hostname renders the machine hostname. MVP module (spec §18). It is effectively
// static, so it has no refresh interval.
type Hostname struct{}

// NewHostname creates a hostname module.
func NewHostname() *Hostname { return &Hostname{} }

func (m *Hostname) ID() status.ModuleID     { return "hostname" }
func (m *Hostname) Interval() time.Duration { return 0 }

// Refresh reads the hostname once.
func (m *Hostname) Refresh(_ context.Context) status.ModuleSnapshot {
	name, err := os.Hostname()
	snap := status.ModuleSnapshot{ID: m.ID(), UpdatedAt: time.Now()}
	if err != nil {
		snap.Err = err
		snap.Value = status.Text("?")
		return snap
	}
	snap.Value = status.Text(name)
	return snap
}
