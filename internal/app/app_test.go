package app

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/hsgiga/ptyline/internal/app/bar"
	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/reserved"
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
		"git": {Enabled: true, Animation: config.AnimationDefault, AnimationIntervalMS: 120},
	})
	if interval != 120*time.Millisecond || continuous {
		t.Fatalf("git change animation = (%v, %t), want (120ms, false)", interval, continuous)
	}

	interval, continuous = bar.TickerConfig(map[string]config.ModuleConfig{
		"time": {Enabled: true, Animation: config.AnimationDefault, AnimationIntervalMS: 120},
	})
	if interval != 0 || continuous {
		t.Fatalf("time animation = (%v, %t), want disabled", interval, continuous)
	}
}

func TestAnimationsFromConfig(t *testing.T) {
	got := bar.AnimationsFromConfig(map[string]config.ModuleConfig{
		"command": {Enabled: true, Animation: config.AnimationDefault},
		"git":     {Enabled: true, Animation: "blink"},
		"time":    {Enabled: true, Animation: config.AnimationDefault},
		"env":     {Enabled: true, Animation: "none"},
	})
	if got["command"].Trigger != "active" || got["command"].Mode != "" {
		t.Fatalf("command animation = %+v, want default active", got["command"])
	}
	if got["git"].Trigger != "change" || got["git"].Mode != "blink" {
		t.Fatalf("git animation = %+v, want blink change", got["git"])
	}
	if _, ok := got["time"]; ok {
		t.Fatalf("time animation unexpectedly present: %+v", got)
	}
	if _, ok := got["env"]; ok {
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

func TestExecEnvNames(t *testing.T) {
	cfg := config.Default()
	cfg.Modules = map[string]config.ModuleConfig{
		"env": {
			Env: []string{"DISPLAY_ONLY"},
		},
		"gh": {
			Source:  "exec",
			Command: "gh api user --jq .login",
			Env:     []string{"GH_HOST", "GH_TOKEN", "PATH"},
		},
		"aws": {
			Command: "aws sts get-caller-identity",
			Env:     []string{"AWS_PROFILE", "PATH"},
		},
		"time_local": {
			Source: "time",
			Env:    []string{"IGNORED"},
		},
	}

	want := []string{"AWS_PROFILE", "GH_HOST", "GH_TOKEN", "PATH"}
	if got := execEnvNames(cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("execEnvNames() = %v, want %v", got, want)
	}
}

func TestExecEnvNamesWildcardAndRejection(t *testing.T) {
	cfg := config.Default()
	cfg.Modules = map[string]config.ModuleConfig{
		"gh": {
			Source:  "exec",
			Command: "gh api user --jq .login",
			// Valid: exact + trailing-* prefix. Invalid: bare *, mid-*, leading
			// digit, non-identifier chars — all dropped.
			Env: []string{"GH_*", "PATH", "*", "G*H", "1BAD", "BAD-NAME"},
		},
	}
	want := []string{"GH_*", "PATH"}
	if got := execEnvNames(cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("execEnvNames() = %v, want %v", got, want)
	}
}

func TestParseExecEnv(t *testing.T) {
	const nonce = "deadbeef"
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	// Value carries ';' and '=' — base64 keeps them from corrupting the frame.
	frame := nonce + ":GH_HOST=" + b64("github.example") + ";GH_TOKEN=" + b64("a;b=c")

	got := parseExecEnv(frame, nonce)
	want := map[string]string{"GH_HOST": "github.example", "GH_TOKEN": "a;b=c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseExecEnv() = %v, want %v", got, want)
	}

	if got := parseExecEnv(frame, "wrongnonce"); got != nil {
		t.Fatalf("parseExecEnv with wrong nonce = %v, want nil", got)
	}
	if got := parseExecEnv("nocolon", nonce); got != nil {
		t.Fatalf("parseExecEnv with malformed frame = %v, want nil", got)
	}
	if got := parseExecEnv(nonce+":", nonce); got == nil || len(got) != 0 {
		t.Fatalf("parseExecEnv empty snapshot = %v, want empty non-nil map", got)
	}
	// A bad base64 entry is skipped, not fatal.
	if got := parseExecEnv(nonce+":GH_HOST=@@@;GH_TOKEN="+b64("ok"), nonce); !reflect.DeepEqual(got, map[string]string{"GH_TOKEN": "ok"}) {
		t.Fatalf("parseExecEnv skipping bad base64 = %v", got)
	}
}

func TestStripNonce(t *testing.T) {
	const nonce = "deadbeef"
	if got, ok := stripNonce(nonce+":/home/u/proj", nonce); !ok || got != "/home/u/proj" {
		t.Fatalf("stripNonce valid = (%q, %v), want (\"/home/u/proj\", true)", got, ok)
	}
	// A path containing ':' survives — only the first segment is the nonce.
	if got, ok := stripNonce(nonce+":/a:b", nonce); !ok || got != "/a:b" {
		t.Fatalf("stripNonce with ':' in path = (%q, %v), want (\"/a:b\", true)", got, ok)
	}
	// Forged frame (no nonce, as an injected OSC 777 would be) is rejected.
	if _, ok := stripNonce("/etc", nonce); ok {
		t.Fatal("stripNonce accepted a frame with no nonce prefix")
	}
	if _, ok := stripNonce("wrong:/etc", nonce); ok {
		t.Fatal("stripNonce accepted a frame with the wrong nonce")
	}
	// An empty configured nonce never matches (defensive; never happens in a real run).
	if _, ok := stripNonce(":/etc", ""); ok {
		t.Fatal("stripNonce accepted an empty nonce")
	}
}

func TestChangedEnvNames(t *testing.T) {
	old := map[string]string{"A": "1", "B": "2", "GONE": "x"}
	next := map[string]string{"A": "1", "B": "changed", "NEW": "y"}
	got := changedEnvNames(old, next)
	sort.Strings(got)
	want := []string{"B", "GONE", "NEW"} // changed, removed, added; unchanged A omitted
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changedEnvNames() = %v, want %v", got, want)
	}
	if got := changedEnvNames(old, old); len(got) != 0 {
		t.Fatalf("changedEnvNames(equal) = %v, want none", got)
	}
}

func TestExecModuleRuntimeCoalescesRefresh(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "count")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := event.NewBus(16)
	m := newExecModuleRuntime("exec", config.ModuleConfig{
		Command:   "sleep 0.1; printf x >> " + counter,
		TimeoutMS: 2000,
	}, nil)

	m.refresh(ctx, bus)               // becomes the worker, then sleeps
	time.Sleep(20 * time.Millisecond) // ensure it is mid-run
	m.refresh(ctx, bus)               // in-flight: must coalesce, not drop

	deadline := time.After(2 * time.Second)
	for {
		if data, _ := os.ReadFile(counter); len(data) >= 2 {
			return // both runs happened
		}
		select {
		case <-deadline:
			data, _ := os.ReadFile(counter)
			t.Fatalf("expected 2 coalesced runs, got %d", len(data))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestExecModuleRuntimeMirrorsAny(t *testing.T) {
	m := newExecModuleRuntime("gh", config.ModuleConfig{
		Command: "printf ok",
		Env:     []string{"GH_*", "PATH"},
	}, nil)
	if !m.mirrorsAny([]string{"AWS_PROFILE", "GH_TOKEN"}) {
		t.Fatal("expected mirrorsAny to match GH_TOKEN via GH_*")
	}
	if m.mirrorsAny([]string{"AWS_PROFILE", "HOME"}) {
		t.Fatal("expected mirrorsAny to ignore unrelated names")
	}
}

func TestEnvNameMatches(t *testing.T) {
	cases := []struct {
		name, pattern string
		want          bool
	}{
		{"GH_TOKEN", "GH_*", true},
		{"GITHUB_TOKEN", "GH_*", false},
		{"PATH", "PATH", true},
		{"PATH2", "PATH", false},
		{"GH_", "GH_*", true},
	}
	for _, c := range cases {
		if got := envNameMatches(c.name, c.pattern); got != c.want {
			t.Errorf("envNameMatches(%q, %q) = %v, want %v", c.name, c.pattern, got, c.want)
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
	}, nil)

	module.refreshAfterCommand(ctx, bus, " gh auth login --web ")

	select {
	case got := <-bus.Events():
		update, ok := got.(event.ModuleUpdated)
		if !ok {
			t.Fatalf("event = %T, want ModuleUpdated", got)
		}
		snap := update.Snapshot
		if snap.ID != "gh" || snap.Value.Text != "ok" {
			t.Fatalf("snapshot = %#v, want gh=ok", snap)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh_on_command did not emit ModuleUpdated")
	}
}

func TestExecModuleRuntimeUsesEnvProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := event.NewBus(4)
	module := newExecModuleRuntime("exec", config.ModuleConfig{
		Command:   `printf '%s:%s' "$A" "$B"`,
		Env:       []string{"A", "B"},
		TimeoutMS: 1000,
	}, &execRuntimeDeps{env: func(names []string) []string {
		if !reflect.DeepEqual(names, []string{"A", "B"}) {
			t.Fatalf("env names = %v, want [A B]", names)
		}
		return []string{"A=one", "B=two"}
	}})

	module.refresh(ctx, bus)

	select {
	case got := <-bus.Events():
		update, ok := got.(event.ModuleUpdated)
		if !ok {
			t.Fatalf("event = %T, want ModuleUpdated", got)
		}
		snap := update.Snapshot
		if snap.ID != "exec" || snap.Value.Text != "one:two" {
			t.Fatalf("snapshot = %#v, want exec=one:two", snap)
		}
	case <-time.After(time.Second):
		t.Fatal("exec refresh did not emit ModuleUpdated")
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
	}, nil)

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
	}, nil)

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
