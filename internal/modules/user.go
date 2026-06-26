package modules

import (
	"context"
	"os"
	osuser "os/user"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// User renders the current local username. It is static for the process
// lifetime and hidden when no username can be resolved.
type User struct {
	value string
}

// NewUser creates a user module, resolving the username immediately.
func NewUser() *User {
	return &User{value: userLabel()}
}

func (m *User) ID() status.ModuleID     { return "user" }
func (m *User) Interval() time.Duration { return 0 }

func (m *User) Refresh(_ context.Context) status.ModuleSnapshot {
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(m.value),
		UpdatedAt: time.Now(),
	}
}

func userLabel() string {
	for _, key := range []string{"USER", "LOGNAME", "USERNAME"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	current, err := osuser.Current()
	if err != nil || current == nil {
		return ""
	}
	return current.Username
}
