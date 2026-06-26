package modules

import (
	"testing"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

func TestGoTimeLayout(t *testing.T) {
	if got := goTimeLayout("%H:%M:%S"); got != "15:04:05" {
		t.Fatalf("goTimeLayout() = %q", got)
	}
}

func TestAbbreviateHome(t *testing.T) {
	if got := AbbreviateHome("/home/user/project", "/home/user"); got != "~/project" {
		t.Fatalf("AbbreviateHome() = %q", got)
	}
}

func TestUserLabelFromEnv(t *testing.T) {
	t.Setenv("USER", "alice")
	t.Setenv("LOGNAME", "")
	t.Setenv("USERNAME", "")

	if got := userLabel(); got != "alice" {
		t.Fatalf("userLabel() = %q, want alice", got)
	}
}

func TestShellLabel(t *testing.T) {
	if got := shellLabel([]string{"/bin/zsh"}); got != "zsh" {
		t.Fatalf("shellLabel() = %q, want zsh", got)
	}
	if got := shellLabel([]string{"-bash"}); got != "bash" {
		t.Fatalf("shellLabel() = %q, want bash", got)
	}
	if got := shellLabel(nil); got != "" {
		t.Fatalf("shellLabel(nil) = %q, want empty", got)
	}
}

func TestEnvValue(t *testing.T) {
	t.Setenv("PTYLINE_TEST_ENV", "staging")

	if got := envValue("PTYLINE_TEST_ENV"); got != "staging" {
		t.Fatalf("envValue() = %q, want staging", got)
	}
	if got := envValue(""); got != "" {
		t.Fatalf("envValue(empty) = %q, want empty", got)
	}

	module := NewEnv([]string{"PTYLINE_TEST_ENV"}, 1500*time.Millisecond)
	if got := module.Interval(); got != 1500*time.Millisecond {
		t.Fatalf("Env.Interval() = %v, want 1500ms", got)
	}
	if got := module.Refresh(nil).Value.Text; got != "staging" {
		t.Fatalf("Env.Refresh() = %q, want staging", got)
	}
}

func TestFormatEnvValues(t *testing.T) {
	lookup := func(name string) string {
		switch name {
		case "APP_ENV":
			return "dev"
		case "REGION":
			return "eu"
		default:
			return ""
		}
	}
	if got := formatEnvValues([]string{"APP_ENV"}, lookup); got != "dev" {
		t.Fatalf("single env = %q, want dev", got)
	}
	if got := formatEnvValues([]string{"APP_ENV", "REGION", "EMPTY"}, lookup); got != "APP_ENV=dev REGION=eu" {
		t.Fatalf("env list = %q, want APP_ENV=dev REGION=eu", got)
	}
}

func TestFormatCommandActiveDoneIdle(t *testing.T) {
	text, active := FormatCommand(status.ShellState{
		ActiveCommand:  "npm test",
		LastCommand:    "git pull",
		LastExitCode:   0,
		LastDurationMS: 1234,
	}, "{active} {last} {exit} {duration}", 60, CommandDisplayPolicy{})
	if text != "npm test" || !active {
		t.Fatalf("active command = (%q, %t), want (npm test, true)", text, active)
	}

	text, active = FormatCommand(status.ShellState{
		LastCommand:          "npm test",
		LastExitCode:         2,
		LastDurationMS:       8420,
		LastCommandCompleted: true,
	}, "{active} {last} {exit} {duration}", 60, CommandDisplayPolicy{})
	if text != "npm test exit 2 8.4s" || active {
		t.Fatalf("done command = (%q, %t), want (npm test exit 2 8.4s, false)", text, active)
	}

	text, active = FormatCommand(status.ShellState{}, "{active} {last} {exit} {duration}", 60, CommandDisplayPolicy{})
	if text != "" || active {
		t.Fatalf("idle command = (%q, %t), want empty false", text, active)
	}
}

func TestFormatCommandTruncates(t *testing.T) {
	text, _ := FormatCommand(status.ShellState{ActiveCommand: "abcdefghijklmnopqrstuvwxyz"}, "{active}", 10, CommandDisplayPolicy{})
	if text != "abcdefghi…" {
		t.Fatalf("truncated command = %q", text)
	}
}

func TestFormatCommandDonePolicy(t *testing.T) {
	completedAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	policy := CommandDisplayPolicy{
		DoneMinDuration: 2 * time.Second,
		DoneSuccessTTL:  3 * time.Second,
		Now:             completedAt.Add(time.Second),
	}
	shortSuccess := status.ShellState{
		LastCommand:            "true",
		LastExitCode:           0,
		LastDurationMS:         1500,
		LastCommandCompleted:   true,
		LastCommandCompletedAt: completedAt,
	}
	text, _ := FormatCommand(shortSuccess, "{last} {exit} {duration}", 60, policy)
	if text != "" {
		t.Fatalf("short success command = %q, want hidden", text)
	}

	longSuccess := shortSuccess
	longSuccess.LastCommand = "sleep 3"
	longSuccess.LastDurationMS = 3100
	text, _ = FormatCommand(longSuccess, "{last} {exit} {duration}", 60, policy)
	if text != "sleep 3 ok 3.1s" {
		t.Fatalf("long success command = %q, want visible", text)
	}
	if !ShouldTickDoneCommand(longSuccess, policy) {
		t.Fatalf("long success should keep ticker alive until TTL")
	}

	policy.Now = completedAt.Add(3 * time.Second)
	text, _ = FormatCommand(longSuccess, "{last} {exit} {duration}", 60, policy)
	if text != "" {
		t.Fatalf("expired success command = %q, want hidden", text)
	}
	if !ShouldClearDoneCommand(longSuccess, policy) {
		t.Fatalf("expired success should clear")
	}

	failure := longSuccess
	failure.LastCommand = "npm test"
	failure.LastExitCode = 2
	text, _ = FormatCommand(failure, "{last} {exit} {duration}", 60, policy)
	if text != "npm test exit 2 3.1s" {
		t.Fatalf("failure command = %q, want visible", text)
	}
	if ShouldClearDoneCommand(failure, policy) {
		t.Fatalf("failure should not auto-clear with default policy")
	}
}
