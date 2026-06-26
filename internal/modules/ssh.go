package modules

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// SSH shows "user@host" when ptyline is running inside an SSH session.
// Detected via $SSH_CLIENT / $SSH_TTY env vars set by sshd. Returns an empty
// value when not in SSH so the block is hidden by the renderer.
type SSH struct {
	value string // resolved once at construction; static for the session lifetime
}

// NewSSH creates the module and resolves user@host immediately. Returns empty
// value when the process is not running inside an SSH session.
func NewSSH() *SSH {
	return &SSH{value: sshLabel()}
}

func (m *SSH) ID() status.ModuleID     { return "ssh" }
func (m *SSH) Interval() time.Duration { return 0 }

func (m *SSH) Refresh(_ context.Context) status.ModuleSnapshot {
	return status.ModuleSnapshot{
		ID:                  m.ID(),
		Value:               status.Text(m.value),
		UpdatedAt:           time.Now(),
		AnimationSuppressed: m.value == "",
	}
}

// SSHEnvLabel returns "user@shorthost" when ptyline itself is running inside an
// SSH session (detected via $SSH_CLIENT / $SSH_TTY set by sshd), or "".
func SSHEnvLabel() string { return sshLabel() }

func sshLabel() string {
	if os.Getenv("SSH_CLIENT") == "" && os.Getenv("SSH_TTY") == "" {
		return ""
	}
	user := os.Getenv("USER")
	host, _ := os.Hostname()
	// Strip domain; keep only the first label so the bar stays compact.
	if dot := strings.IndexByte(host, '.'); dot > 0 {
		host = host[:dot]
	}
	if user == "" && host == "" {
		return ""
	}
	if user == "" {
		return host
	}
	if host == "" {
		return user
	}
	return user + "@" + host
}
