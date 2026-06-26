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

// sshConnectingAnimationTimeout is how long SSHAnimator animates after
// ssh_start before settling into a static label (covers TCP handshake + key
// exchange window).
const sshConnectingAnimationTimeout = 2500 * time.Millisecond

// SSHAnimator tracks the outbound-SSH animation lifecycle: a brief connecting
// window then static suppression. It is created once and called from the event
// loop goroutine; no locking is needed.
type SSHAnimator struct {
	animating bool
	lastStart time.Time
	baseSnap  status.ModuleSnapshot
}

// NewSSHAnimator creates an animator using base as the snapshot to restore when
// the SSH session ends or the connecting window expires.
func NewSSHAnimator(base status.ModuleSnapshot) *SSHAnimator {
	return &SSHAnimator{baseSnap: base}
}

// OnSSHStart records the ssh_start event and returns the animated snapshot to
// publish.
func (a *SSHAnimator) OnSSHStart(target string) status.ModuleSnapshot {
	a.animating = true
	a.lastStart = time.Now()
	return status.ModuleSnapshot{
		ID:        "ssh",
		Value:     status.Text(target),
		UpdatedAt: time.Now(),
	}
}

// OnSSHEnd records the ssh_end event and returns the base snapshot to restore.
func (a *SSHAnimator) OnSSHEnd() status.ModuleSnapshot {
	a.animating = false
	return a.baseSnap
}

// Tick checks whether the connecting window has expired; returns a non-nil
// snapshot when the animation should be suppressed. Must be called on every
// Tick event.
func (a *SSHAnimator) Tick(target string) *status.ModuleSnapshot {
	if !a.animating {
		return nil
	}
	if time.Since(a.lastStart) <= sshConnectingAnimationTimeout {
		return nil
	}
	a.animating = false
	snap := status.ModuleSnapshot{
		ID:                  "ssh",
		Value:               status.Text(target),
		UpdatedAt:           time.Now(),
		AnimationSuppressed: true,
	}
	return &snap
}

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
