package command

import (
	"testing"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/status"
)

func TestTrackerKeepsSilentCommandAnimatingDuringStartGrace(t *testing.T) {
	tracker := NewTracker(config.ModuleConfig{Enabled: true})
	now := time.Now()
	tracker.startedAt = now.Add(-commandAnimationStartGrace + 100*time.Millisecond)
	tracker.lastActivity = tracker.startedAt

	st := status.NewState()
	st.Shell.ActiveCommand = "sleep 2"
	st.Shell.LastCommand = "sleep 2"

	snap := tracker.Tick(&st)
	if snap == nil {
		t.Fatal("Tick() returned nil snapshot")
	}
	if st.ActiveCommandAnimating != true || snap.AnimationSuppressed {
		t.Fatalf("silent command during grace should animate: state=%+v snap=%+v", st, snap)
	}
	if !tracker.Animating().Load() {
		t.Fatal("ticker flag should stay active during start grace")
	}
}

func TestTrackerSuppressesSilentCommandAfterStartGrace(t *testing.T) {
	tracker := NewTracker(config.ModuleConfig{Enabled: true})
	now := time.Now()
	tracker.startedAt = now.Add(-commandAnimationStartGrace - 100*time.Millisecond)
	tracker.lastActivity = now.Add(-commandAnimationStartGrace - commandAnimationIdleTimeout)
	tracker.animating.Store(true)

	st := status.NewState()
	st.Shell.ActiveCommand = "sleep 60"
	st.Shell.LastCommand = "sleep 60"

	snap := tracker.Tick(&st)
	if snap == nil {
		t.Fatal("Tick() returned nil snapshot")
	}
	if st.ActiveCommandAnimating || !snap.AnimationSuppressed {
		t.Fatalf("silent command after grace should be idle: state=%+v snap=%+v", st, snap)
	}
	if tracker.Animating().Load() {
		t.Fatal("ticker flag should stop after silent command exceeds grace")
	}
}

func TestTrackerHidesActiveCommandDuringAppearanceGrace(t *testing.T) {
	tracker := NewTracker(config.ModuleConfig{Enabled: true})

	st := status.NewState()
	st.ApplyShellMeta("command", "ls")

	snap := tracker.ApplyShellMeta("command", &st)
	if snap != nil {
		t.Fatalf("active command within grace should not emit a snapshot, got %+v", snap)
	}
	if st.ActiveCommandAnimating {
		t.Fatal("command should not animate before the appearance grace elapses")
	}
	if !tracker.Animating().Load() {
		t.Fatal("ticker flag must stay active so a Tick can reveal the command")
	}

	// A duration_ms / exit_code snapshot arriving while still running must also
	// keep the name hidden (it routes through snapshot(), not the command branch).
	st.ApplyShellMeta("duration_ms", "5")
	if snap := tracker.ApplyShellMeta("duration_ms", &st); snap == nil || snap.Value.Text != "" {
		t.Fatalf("active name must stay hidden on a mid-run snapshot, got %+v", snap)
	}
}

func TestTrackerRevealsActiveCommandAfterGrace(t *testing.T) {
	tracker := NewTracker(config.ModuleConfig{Enabled: true})
	st := status.NewState()
	st.ApplyShellMeta("command", "make build")
	_ = tracker.ApplyShellMeta("command", &st)

	// Simulate the grace having elapsed, then a Tick reveals the command.
	tracker.startedAt = time.Now().Add(-tracker.ActiveShowDelay() - time.Millisecond)

	snap := tracker.Tick(&st)
	if snap == nil {
		t.Fatal("Tick after grace should reveal the active command")
	}
	if snap.Value.Text == "" {
		t.Fatalf("revealed command snapshot should carry the name, got %+v", snap)
	}
	if !st.ActiveCommandAnimating {
		t.Fatal("revealed command should animate")
	}
}

func TestTrackerNeverShowsCommandThatFinishesWithinGrace(t *testing.T) {
	tracker := NewTracker(config.ModuleConfig{Enabled: true})
	st := status.NewState()

	st.ApplyShellMeta("command", "cd /tmp")
	if snap := tracker.ApplyShellMeta("command", &st); snap != nil {
		t.Fatalf("start within grace should be silent, got %+v", snap)
	}

	// Command completes almost immediately, well inside the grace.
	st.ApplyShellMeta("exit_code", "0")
	st.ApplyShellMeta("duration_ms", "12")
	st.ApplyShellMeta("command", "")
	snap := tracker.ApplyShellMeta("command", &st)
	if snap == nil {
		t.Fatal("completion should emit a snapshot to settle the module")
	}
	if snap.Value.Text != "" {
		t.Fatalf("instant command must never appear in the bar, got %q", snap.Value.Text)
	}
}

func TestCommandSpansDurationBeforeExit(t *testing.T) {
	// Default config format "{active} {last} | {duration} | {exit}" renders as
	// "sleep 5 • 1s • sigint" — exit appears AFTER duration in the string.
	// Both spans must be found regardless of order.
	spans := commandSpans("sleep 5 • 1s • sigint", status.ShellState{
		LastCommand:          "sleep 5",
		LastExitCode:         130,
		LastDurationMS:       1000,
		LastCommandCompleted: true,
	})
	foundExit, foundDuration := false, false
	for _, s := range spans {
		if s.Role == "exit" && s.Text == "sigint" {
			foundExit = true
		}
		if s.Role == "duration" && s.Text == "1s" {
			foundDuration = true
		}
	}
	if !foundExit {
		t.Fatalf("exit span missing when duration precedes exit in text; spans=%+v", spans)
	}
	if !foundDuration {
		t.Fatalf("duration span missing; spans=%+v", spans)
	}
}

func TestCommandSpansPreferExitNearEnd(t *testing.T) {
	spans := commandSpans("echo ok • ok • 1s", status.ShellState{
		LastCommand:          "echo ok",
		LastExitCode:         0,
		LastDurationMS:       1000,
		LastCommandCompleted: true,
	})
	if len(spans) < 4 {
		t.Fatalf("spans = %+v, want split command/exit/duration", spans)
	}
	if spans[0].Text != "echo ok • " {
		t.Fatalf("first span = %+v, should leave command text unstyled", spans[0])
	}
	if spans[1].Text != "ok" || spans[1].Role != "exit" {
		t.Fatalf("exit span = %+v, want trailing ok as exit", spans[1])
	}
}
