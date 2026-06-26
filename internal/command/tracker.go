// Package command owns the command-animation lifecycle state machine: it tracks
// whether the running command is "working" (producing output that is not just
// a keystroke echo), enforces the idle timeout, and drives the done-command TTL
// tick-down. All policy constants live here so app.go stays wiring-only.
package command

import (
	"sync/atomic"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/modules"
	"github.com/hsgiga/ptyline/internal/shellintegration"
	"github.com/hsgiga/ptyline/internal/status"
)

// commandAnimationIdleTimeout stops the glint for interactive commands that
// remain active but stop producing output, such as an idle agent prompt.
const commandAnimationIdleTimeout = 1200 * time.Millisecond

// keystrokeEchoWindow is how long after a keystroke output is treated as the
// program echoing the user's typing rather than doing work; within it the
// command glint does not start.
const keystrokeEchoWindow = 180 * time.Millisecond

// Tracker owns the command-animation lifecycle. It is created once and called
// exclusively from the event-loop goroutine (except Animating, which may be
// read by the ticker goroutine).
type Tracker struct {
	animating    atomic.Bool
	lastActivity time.Time
	lastStdin    time.Time
	cfg          config.ModuleConfig
}

// NewTracker creates a Tracker from the command module config.
func NewTracker(cfg config.ModuleConfig) *Tracker {
	return &Tracker{cfg: cfg}
}

// Animating returns the atomic flag readable by the ticker goroutine.
func (t *Tracker) Animating() *atomic.Bool {
	return &t.animating
}

// RecordKeystroke marks the most recent user keypress.
func (t *Tracker) RecordKeystroke() {
	t.lastStdin = time.Now()
}

// RecordOutput counts output bytes as "work" when they are not just a
// keystroke echo. Returns true when the animating flag changed.
func (t *Tracker) RecordOutput(shell *status.ShellState) bool {
	if shell.ActiveCommand == "" {
		return false
	}
	if time.Since(t.lastStdin) <= keystrokeEchoWindow {
		return false
	}
	t.lastActivity = time.Now()
	if t.animating.Load() {
		return false
	}
	t.animating.Store(true)
	return true
}

// ApplyShellMeta updates the tracker when a shell-integration key fires.
// Returns a ModuleSnapshot to publish (nil = no change needed).
func (t *Tracker) ApplyShellMeta(key string, st *status.StatusState) *status.ModuleSnapshot {
	if key != shellintegration.KeyCommand && key != "exit_code" && key != "duration_ms" {
		return nil
	}
	if key == shellintegration.KeyCommand {
		if st.Shell.ActiveCommand != "" {
			t.lastActivity = time.Now()
			st.ActiveCommandAnimating = true
			t.animating.Store(true)
		} else {
			st.AnimationPhase = 0
			st.ActiveCommandAnimating = false
			t.animating.Store(modules.ShouldTickDoneCommand(st.Shell, DisplayPolicy(t.cfg)))
		}
	}
	if !t.cfg.Enabled {
		return nil
	}
	snap := t.snapshot(st.Shell, st.ActiveCommandAnimating)
	return &snap
}

// Tick advances the state on every animation tick and returns a ModuleSnapshot
// to publish (nil = no change needed).
func (t *Tracker) Tick(st *status.StatusState) *status.ModuleSnapshot {
	if st.Shell.ActiveCommand == "" {
		st.ActiveCommandAnimating = false
		if modules.ShouldClearDoneCommand(st.Shell, DisplayPolicy(t.cfg)) {
			st.Shell.ClearLastCommand()
			t.animating.Store(false)
			if t.cfg.Enabled {
				snap := t.snapshot(st.Shell, false)
				return &snap
			}
		}
		return nil
	}
	if time.Since(t.lastActivity) > commandAnimationIdleTimeout {
		st.ActiveCommandAnimating = false
		t.animating.Store(false)
		if t.cfg.Enabled {
			snap := t.snapshot(st.Shell, false)
			return &snap
		}
		return nil
	}
	st.ActiveCommandAnimating = true
	if t.cfg.Enabled {
		snap := t.snapshot(st.Shell, true)
		return &snap
	}
	return nil
}

// DisplayPolicy converts command module config into a CommandDisplayPolicy.
func DisplayPolicy(cfg config.ModuleConfig) modules.CommandDisplayPolicy {
	return modules.CommandDisplayPolicy{
		DoneMinDuration: time.Duration(cfg.DoneMinDurationMS) * time.Millisecond,
		DoneSuccessTTL:  time.Duration(cfg.DoneSuccessTTLMS) * time.Millisecond,
		DoneFailureTTL:  time.Duration(cfg.DoneFailureTTLMS) * time.Millisecond,
	}
}

func (t *Tracker) snapshot(shell status.ShellState, animating bool) status.ModuleSnapshot {
	text, active := modules.FormatCommand(shell, t.cfg.Format, t.cfg.MaxWidth, DisplayPolicy(t.cfg))
	return status.ModuleSnapshot{
		ID:                  "command",
		Value:               status.Text(text),
		UpdatedAt:           time.Now(),
		AnimationSuppressed: !active || !animating,
	}
}
