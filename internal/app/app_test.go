package app

import (
	"context"
	"testing"
	"time"

	"github.com/hsgiga/ptyline/internal/app/bar"
	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/status"
)

func TestBarGeometry(t *testing.T) {
	cases := []struct {
		name      string
		area      reserved.Area
		rows      uint16
		wantRows  int
		wantTop   uint16
		wantCount int
	}{
		{
			name:      "single bottom row",
			area:      reserved.Default(),
			rows:      30,
			wantRows:  1,
			wantTop:   30,
			wantCount: 1,
		},
		{
			name:      "multi row bar",
			area:      reserved.Area{Edge: reserved.Bottom, Rows: 2},
			rows:      30,
			wantRows:  2,
			wantTop:   29,
			wantCount: 2,
		},
		{
			name:      "tiny terminal drops overflowing bar rows",
			area:      reserved.Area{Edge: reserved.Bottom, Rows: 2},
			rows:      1,
			wantRows:  2,
			wantTop:   2,
			wantCount: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			top, count := bar.Geometry(c.area, c.rows, c.wantRows)
			if top != c.wantTop || count != c.wantCount {
				t.Fatalf("bar.Geometry(%+v, %d, %d) = (%d, %d), want (%d, %d)",
					c.area, c.rows, c.wantRows, top, count, c.wantTop, c.wantCount)
			}
		})
	}
}

func TestAnimationTickerConfig(t *testing.T) {
	interval, continuous := bar.TickerConfig(map[string]config.ModuleConfig{
		"command": {Enabled: true, Animation: "glint", AnimationIntervalMS: 80},
	})
	if interval != 80*time.Millisecond || continuous {
		t.Fatalf("command animation = (%v, %t), want (80ms, false)", interval, continuous)
	}

	interval, continuous = bar.TickerConfig(map[string]config.ModuleConfig{
		"time": {Enabled: true, Animation: "glint", AnimationIntervalMS: 120},
	})
	if interval != 120*time.Millisecond || !continuous {
		t.Fatalf("time animation = (%v, %t), want (120ms, true)", interval, continuous)
	}
}

func TestAnimationsFromConfig(t *testing.T) {
	got := bar.AnimationsFromConfig(map[string]config.ModuleConfig{
		"time": {Enabled: true, Animation: "glint"},
		"git":  {Enabled: true, Animation: "none"},
	})
	if got["time"].Mode != "glint" {
		t.Fatalf("time animation = %+v, want glint", got["time"])
	}
	if _, ok := got["git"]; ok {
		t.Fatalf("disabled animation unexpectedly present: %+v", got)
	}
}

func TestModuleSource(t *testing.T) {
	if got := config.ModuleSource("gh", config.ModuleConfig{}); got != "exec" {
		t.Fatalf("unknown module source = %q, want exec", got)
	}
	if got := config.ModuleSource("time", config.ModuleConfig{}); got != "" {
		t.Fatalf("builtin time source = %q, want builtin empty source", got)
	}
	if got := config.ModuleSource("time_local", config.ModuleConfig{Source: "time"}); got != "time" {
		t.Fatalf("explicit source = %q, want time", got)
	}
	if got := config.ModuleSource("kube", config.ModuleConfig{Provider: "command"}); got != "exec" {
		t.Fatalf("provider command source = %q, want exec", got)
	}
}

func TestGitBranchIcon(t *testing.T) {
	if got := gitBranchIcon("nerd-font"); got != "" {
		t.Fatalf("nerd-font git icon = %q, want %q", got, "")
	}
	for _, preset := range []string{"ascii", "emoji", ""} {
		if got := gitBranchIcon(preset); got != "⎇" {
			t.Fatalf("%s git icon = %q, want %q", preset, got, "⎇")
		}
	}
}

func TestCommandMatches(t *testing.T) {
	tests := []struct {
		actual  string
		pattern string
		want    bool
	}{
		{actual: "gh auth login", pattern: "gh auth login", want: true},
		{actual: " gh  auth   login --web ", pattern: "gh auth login", want: true},
		{actual: "gh auth login2", pattern: "gh auth login", want: false},
		{actual: "gh auth logout", pattern: "gh auth login", want: false},
		{actual: "", pattern: "gh auth login", want: false},
		{actual: "gh auth login", pattern: " ", want: false},
	}
	for _, tc := range tests {
		if got := commandMatches(tc.actual, tc.pattern); got != tc.want {
			t.Fatalf("commandMatches(%q, %q) = %t, want %t", tc.actual, tc.pattern, got, tc.want)
		}
	}
}

func TestExecModuleRuntimeRefreshAfterCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := event.NewBus(4)
	module := newExecModuleRuntime("gh", config.ModuleConfig{
		Command:          "printf ok",
		RefreshOnCommand: []string{"gh auth login"},
		TimeoutMS:        1000,
	})

	module.refreshAfterCommand(ctx, bus, " gh auth login --web ")

	select {
	case got := <-bus.Events():
		update, ok := got.(event.ModuleUpdated)
		if !ok {
			t.Fatalf("event = %T, want ModuleUpdated", got)
		}
		snap, ok := update.Snapshot.(status.ModuleSnapshot)
		if !ok || snap.ID != "gh" || snap.Value.Text != "ok" {
			t.Fatalf("snapshot = %#v, want gh=ok", update.Snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh_on_command did not emit ModuleUpdated")
	}
}

func TestExecModuleRuntimeRefreshAfterCommandNoMatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := event.NewBus(4)
	module := newExecModuleRuntime("gh", config.ModuleConfig{
		Command:          "printf ok",
		RefreshOnCommand: []string{"gh auth login"},
		TimeoutMS:        1000,
	})

	module.refreshAfterCommand(ctx, bus, "gh pr list")

	select {
	case got := <-bus.Events():
		t.Fatalf("unexpected event: %#v", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestExecModuleRuntimeRefreshCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bus := event.NewBus(4)
	module := newExecModuleRuntime("gh", config.ModuleConfig{
		Command:          "printf ok",
		RefreshOnCommand: []string{"gh auth login"},
		TimeoutMS:        1000,
	})

	module.refreshAfterCommand(ctx, bus, "gh auth login")

	select {
	case got := <-bus.Events():
		t.Fatalf("unexpected event: %#v", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestExitCodeSuccess(t *testing.T) {
	if !exitCodeSuccess("0") {
		t.Fatal("exitCodeSuccess(0) = false, want true")
	}
	for _, value := range []string{"1", "2", "", "not-a-code"} {
		if exitCodeSuccess(value) {
			t.Fatalf("exitCodeSuccess(%q) = true, want false", value)
		}
	}
}

func TestShouldRefreshAfterExit(t *testing.T) {
	if !shouldRefreshAfterExit("0", "gh auth login", "gh auth login") {
		t.Fatal("matching successful command should refresh")
	}
	tests := []struct {
		name           string
		exitCode       string
		pendingCommand string
		lastCommand    string
	}{
		{name: "nonzero", exitCode: "1", pendingCommand: "gh auth login", lastCommand: "gh auth login"},
		{name: "stale last command", exitCode: "0", pendingCommand: "", lastCommand: "gh auth login"},
		{name: "different command", exitCode: "0", pendingCommand: "gh auth logout", lastCommand: "gh auth login"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if shouldRefreshAfterExit(tc.exitCode, tc.pendingCommand, tc.lastCommand) {
				t.Fatal("shouldRefreshAfterExit returned true, want false")
			}
		})
	}
}
