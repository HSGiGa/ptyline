// Package command owns the command-animation lifecycle state machine: it tracks
// whether the running command is "working" (producing output that is not just
// a keystroke echo), enforces the idle timeout, and drives the done-command TTL
// tick-down. All policy constants live here so app.go stays wiring-only.
package command

import (
	"sort"
	"strings"
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

// commandAnimationStartGrace keeps a newly-started command looking active even
// before it produces output. This covers short silent commands such as `sleep 2`
// without letting long idle processes animate forever.
const commandAnimationStartGrace = 3 * time.Second

// keystrokeEchoWindow is how long after a keystroke output is treated as the
// program echoing the user's typing rather than doing work; within it the
// command glint does not start.
const keystrokeEchoWindow = 180 * time.Millisecond

// commandActiveShowDelay suppresses the active-command display until the command
// has been running this long, so instant commands (ls, cd, git status) never
// flash into the bar and straight back out. Mirrors DoneMinDuration for the done
// state; overridable via module config active_min_duration_ms.
const commandActiveShowDelay = 200 * time.Millisecond

// Tracker owns the command-animation lifecycle. It is created once and called
// exclusively from the event-loop goroutine (except Animating, which may be
// read by the ticker goroutine).
type Tracker struct {
	animating    atomic.Bool
	startedAt    time.Time
	lastActivity time.Time
	lastStdin    time.Time
	// activeShown reports whether the running command has cleared the appearance
	// grace and is now rendered. Reset on every new command; flipped by Tick once
	// commandActiveShowDelay has elapsed.
	activeShown bool
	cfg         config.ModuleConfig
}

// NewTracker creates a Tracker from the command module config.
func NewTracker(cfg config.ModuleConfig) *Tracker {
	return &Tracker{cfg: cfg}
}

// Animating returns the atomic flag readable by the ticker goroutine.
func (t *Tracker) Animating() *atomic.Bool {
	return &t.animating
}

// ActiveShowDelay is how long a command must run before its name appears in the
// bar. Commands shorter than this never render, killing the flash on instant
// commands. A configured active_min_duration_ms of 0 falls back to the default.
func (t *Tracker) ActiveShowDelay() time.Duration {
	if t.cfg.ActiveMinDurationMS > 0 {
		return time.Duration(t.cfg.ActiveMinDurationMS) * time.Millisecond
	}
	return commandActiveShowDelay
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
			now := time.Now()
			t.startedAt = now
			t.lastActivity = now
			t.activeShown = false
			t.animating.Store(true)
			if t.ActiveShowDelay() > 0 {
				// Hold the name back through the appearance grace; a later Tick
				// reveals it (and instant commands complete first, never shown).
				st.ActiveCommandAnimating = false
				return nil
			}
			t.activeShown = true
			st.ActiveCommandAnimating = true
		} else {
			st.AnimationPhase = 0
			st.ActiveCommandAnimating = false
			t.activeShown = false
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
	now := time.Now()
	if !t.activeShown {
		if now.Sub(t.startedAt) < t.ActiveShowDelay() {
			return nil // still within the appearance grace: stay hidden
		}
		t.activeShown = true
	}
	if !t.startedAt.IsZero() && now.Sub(t.startedAt) <= commandAnimationStartGrace {
		st.ActiveCommandAnimating = true
		t.animating.Store(true)
		if t.cfg.Enabled {
			snap := t.snapshot(st.Shell, true)
			return &snap
		}
		return nil
	}
	if now.Sub(t.lastActivity) > commandAnimationIdleTimeout {
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
		Separator:       cfg.Separator,
	}
}

func (t *Tracker) snapshot(shell status.ShellState, animating bool) status.ModuleSnapshot {
	// During the appearance grace the active name must not render, even when this
	// snapshot is emitted by an unrelated key (exit_code/duration_ms) that arrives
	// while the command is still running. The command has not completed, so
	// blanking ActiveCommand leaves the module empty rather than showing a stale
	// done state.
	if !t.activeShown && shell.ActiveCommand != "" {
		shell.ActiveCommand = ""
	}
	text, active := modules.FormatCommand(shell, t.cfg.Format, t.cfg.MaxWidth, DisplayPolicy(t.cfg))
	return status.ModuleSnapshot{
		ID:                  "command",
		Value:               status.Text(text),
		UpdatedAt:           time.Now(),
		Spans:               commandSpans(text, shell),
		AnimationSuppressed: !active || !animating,
	}
}

func commandSpans(text string, shell status.ShellState) []status.TextSpan {
	if text == "" || shell.ActiveCommand != "" || !shell.LastCommandCompleted {
		return nil
	}
	exit := modules.FormatExit(shell.LastExitCode)
	duration := modules.FormatDuration(shell.LastDurationMS)
	marks := []spanMark{}
	if exit != "" {
		if idx := strings.LastIndex(text, exit); idx >= 0 {
			level := status.LevelError
			if shell.LastExitCode == 0 {
				level = status.LevelOK
			}
			marks = append(marks, spanMark{Start: idx, End: idx + len(exit), Span: status.TextSpan{Text: exit, Role: "exit", Level: level}})
		}
	}
	if duration != "" {
		if idx := strings.LastIndex(text, duration); idx >= 0 {
			marks = append(marks, spanMark{Start: idx, End: idx + len(duration), Span: status.TextSpan{Text: duration, Role: "duration"}})
		}
	}
	if len(marks) == 0 {
		return nil
	}
	sort.Slice(marks, func(i, j int) bool { return marks[i].Start < marks[j].Start })
	spans := []status.TextSpan{}
	cursor := 0
	for _, mark := range marks {
		if mark.Start < cursor {
			continue
		}
		if mark.Start > cursor {
			spans = append(spans, status.TextSpan{Text: text[cursor:mark.Start]})
		}
		spans = append(spans, mark.Span)
		cursor = mark.End
	}
	if cursor < len(text) {
		spans = append(spans, status.TextSpan{Text: text[cursor:]})
	}
	return spans
}

type spanMark struct {
	Start int
	End   int
	Span  status.TextSpan
}
