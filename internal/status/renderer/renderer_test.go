package renderer

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/layout"
	"github.com/hsgiga/ptyline/internal/status/style"
	"github.com/hsgiga/ptyline/internal/status/theme"
	"github.com/hsgiga/ptyline/internal/status/width"
)

// A module value renders into the bar line, and the line never contains a newline
// (which would scroll the bar into scrollback — spec §8.6).
func TestRenderModuleValueNoNewline(t *testing.T) {
	st := status.NewState()
	st.Resize(20, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "time", Value: status.Text("12:00")})

	r := New(layout.New(20), nil)
	out := r.Render(st, layout.ParseFormat("{time}"))

	if !strings.Contains(out.Line, "12:00") {
		t.Fatalf("rendered line %q missing module value", out.Line)
	}
	if strings.Contains(out.Line, "\n") {
		t.Fatalf("rendered line contains a newline: %q", out.Line)
	}
}

// Left/center/right sections render in order across the bar (spec §20.13).
func TestRenderThreeSectionOrder(t *testing.T) {
	st := status.NewState()
	st.Resize(20, 1, false)

	r := New(layout.New(20), nil)
	out := r.Render(st, layout.ParseFormat("L||C||R"))

	l, c, rr := strings.Index(out.Line, "L"), strings.Index(out.Line, "C"), strings.Index(out.Line, "R")
	if l < 0 || c < 0 || rr < 0 || !(l < c && c < rr) {
		t.Fatalf("section order wrong in %q (L=%d C=%d R=%d)", out.Line, l, c, rr)
	}
}

// A border row ('-' fill) fills the whole width with dashes, draws edge caps, and
// places the block in its slot — exactly barWidth cells wide.
func TestRenderRowBorderFill(t *testing.T) {
	st := status.NewState()
	st.Resize(30, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("main")})

	r := New(layout.New(30), theme.Default(theme.NoColor))
	line := r.RenderRow(st, layout.ParseFormat("|| {git} ||"), '-').Line

	if w := width.String(line); w != 30 {
		t.Fatalf("border row width = %d, want 30: %q", w, line)
	}
	if !strings.Contains(line, "main") {
		t.Fatalf("border row missing block value: %q", line)
	}
	if !strings.HasPrefix(line, "--") || !strings.HasSuffix(line, "--") {
		t.Fatalf("border row missing edge caps: %q", line)
	}
	if strings.Contains(line, "\n") {
		t.Fatalf("border row contains a newline: %q", line)
	}
}

func TestRenderRowBorderFillEmptyModuleUsesFill(t *testing.T) {
	st := status.NewState()
	st.Resize(30, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("")})

	r := New(layout.New(30), theme.Default(theme.NoColor))
	line := r.RenderRow(st, layout.ParseFormat("|| {git} ||"), '─').Line

	if w := width.String(line); w != 30 {
		t.Fatalf("border row width = %d, want 30: %q", w, line)
	}
	if strings.Contains(line, " ") {
		t.Fatalf("empty git left a whitespace hole in border row: %q", line)
	}
	if want := strings.Repeat("─", 30); line != want {
		t.Fatalf("empty git border row = %q, want %q", line, want)
	}
}

func TestRenderMainBarHidesEmptyModuleBlock(t *testing.T) {
	st := status.NewState()
	st.Resize(20, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("")})

	blocks := layout.ParseFormat("{git}")
	blocks[0].StyleID = "git"
	r := New(layout.New(20), theme.Default(theme.NoColor))
	r.SetStyles(map[string]style.Style{
		"git": {PaddingLeft: 2, PaddingRight: 2},
	})
	line := r.RenderRow(st, blocks, ' ').Line

	if line != strings.Repeat(" ", 20) {
		t.Fatalf("empty main-bar git block rendered %q, want blank bar", line)
	}
}

func TestActiveCommandAlias(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.Shell.ActiveCommand = "go test ./..."
	st.UpdateModule(status.ModuleSnapshot{ID: "active_command", Value: status.Text("go test ./...")})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	blocks := layout.ParseFormat("{cmd}")

	line := r.RenderRow(st, blocks, ' ').Line

	if !strings.Contains(line, "go test ./...") {
		t.Fatalf("active command alias did not render command: %q", line)
	}
}

func TestActiveCommandGlintKeepsVisibleText(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.Shell.ActiveCommand = "sleep 30"
	st.AnimationPhase = 1
	st.ActiveCommandAnimating = true
	st.UpdateModule(status.ModuleSnapshot{ID: "active_command", Value: status.Text("sleep 30")})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetAnimations(map[string]Animation{"active_command": {Mode: "glint"}})
	line := r.RenderRow(st, layout.ParseFormat("{cmd}"), ' ').Line

	if !strings.Contains(line, "\x1b[38;2;255;240;194m") {
		t.Fatalf("glint output missing highlight color: %q", line)
	}
	if !strings.Contains(stripANSI(line), "sleep 30") {
		t.Fatalf("glint changed visible text: %q", line)
	}
}

func TestActiveCommandGlintStopsWhenIdle(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.Shell.ActiveCommand = "codex"
	st.ActiveCommandAnimating = false
	st.UpdateModule(status.ModuleSnapshot{ID: "active_command", Value: status.Text("codex")})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetAnimations(map[string]Animation{"active_command": {Mode: "glint"}})
	line := r.RenderRow(st, layout.ParseFormat("{cmd}"), ' ').Line

	if strings.Contains(line, "\x1b[38;2;255;240;194m") {
		t.Fatalf("idle active command should not glint: %q", line)
	}
	if !strings.Contains(stripANSI(line), "codex") {
		t.Fatalf("idle active command missing visible text: %q", line)
	}
}

func TestAnyModuleCanGlint(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.AnimationPhase = 1
	st.UpdateModule(status.ModuleSnapshot{ID: "time", Value: status.Text("12:34")})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetAnimations(map[string]Animation{"time": {Mode: "glint"}})
	line := r.RenderRow(st, layout.ParseFormat("{time}"), ' ').Line

	if !strings.Contains(line, "\x1b[38;2;255;240;194m") {
		t.Fatalf("animated time block missing glint highlight: %q", line)
	}
	if !strings.Contains(stripANSI(line), "12:34") {
		t.Fatalf("animated time block changed visible text: %q", line)
	}
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}
