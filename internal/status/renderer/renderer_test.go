package renderer

import (
	"regexp"
	"strings"
	"sync/atomic"
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

func TestRenderSanitizesModuleTerminalControls(t *testing.T) {
	st := status.NewState()
	st.Resize(80, 1, false)
	st.UpdateModule(status.ModuleSnapshot{
		ID:    "env",
		Value: status.Text("ok\x1b]52;c;bad\x07\nnext\u009b31m"),
	})

	r := New(layout.New(80), nil)
	out := r.Render(st, layout.ParseFormat("pre {env}"))

	// Module values must have terminal controls stripped; the literal prefix is trusted config.
	if strings.ContainsAny(out.Line, "\x1b\a\n") || strings.ContainsRune(out.Line, '\u009b') {
		t.Fatalf("rendered line contains terminal controls from module value: %q", out.Line)
	}
	if !strings.Contains(out.Line, "ok]52;c;badnext31m") {
		t.Fatalf("sanitized module value mismatch: %q", out.Line)
	}
}

func TestRenderModuleIconLeftRight(t *testing.T) {
	st := status.NewState()
	st.Resize(80, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("main")})
	st.UpdateModule(status.ModuleSnapshot{ID: "time", Value: status.Text("12:00")})

	r := New(layout.New(80), nil)
	r.SetIcons(map[string]ModuleIcon{
		"git":  {Position: "left", Text: "G"},
		"time": {Position: "right", Text: "T"},
	})
	out := stripANSI(r.Render(st, layout.ParseFormat("{git} {time}")).Line)
	if !strings.Contains(out, "G main") {
		t.Fatalf("left icon missing: %q", out)
	}
	if !strings.Contains(out, "12:00 T") {
		t.Fatalf("right icon missing: %q", out)
	}
}

func TestRenderModuleIconHiddenForEmptyValue(t *testing.T) {
	st := status.NewState()
	st.Resize(20, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("")})

	r := New(layout.New(20), nil)
	r.SetIcons(map[string]ModuleIcon{"git": {Position: "left", Text: "G"}})
	out := stripANSI(r.Render(st, layout.ParseFormat("{git}")).Line)
	if strings.Contains(out, "G") {
		t.Fatalf("empty module rendered lone icon: %q", out)
	}
}

// Left/center/right sections render in order across the bar (spec §20.13).
func TestRenderThreeSectionOrder(t *testing.T) {
	st := status.NewState()
	st.Resize(20, 1, false)

	r := New(layout.New(20), nil)
	out := r.Render(st, layout.ParseFormat("L||C||R"))

	l, c, rr := strings.Index(out.Line, "L"), strings.Index(out.Line, "C"), strings.Index(out.Line, "R")
	if l < 0 || c < 0 || rr < 0 || l >= c || c >= rr {
		t.Fatalf("section order wrong in %q (L=%d C=%d R=%d)", out.Line, l, c, rr)
	}
}

func TestRenderJustifyModes(t *testing.T) {
	st := status.NewState()
	st.Resize(20, 1, false)
	blocks := layout.ParseFormat("L||CC||RRRRRR")

	tests := []struct {
		name    string
		justify Justify
		want    string
	}{
		{name: "center", justify: JustifyCenter, want: "L     CC      RRRRRR"},
		{name: "absolute center", justify: JustifyAbsoluteCenter, want: "L        CC   RRRRRR"},
		{name: "left", justify: JustifyLeft, want: "LCC           RRRRRR"},
		{name: "right", justify: JustifyRight, want: "L           CCRRRRRR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(layout.New(20), nil)
			r.SetJustify(tt.justify)
			line := r.Render(st, blocks).Line
			if line != tt.want {
				t.Fatalf("line = %q, want %q", line, tt.want)
			}
		})
	}
}

func TestRenderAbsoluteCenterFallsBackWhenOverlapping(t *testing.T) {
	st := status.NewState()
	st.Resize(20, 1, false)

	r := New(layout.New(20), nil)
	r.SetJustify(JustifyAbsoluteCenter)
	line := r.Render(st, layout.ParseFormat("LLLLLLLLLL||CC||RRRRRR")).Line

	if want := "LLLLLLLLLL CC RRRRRR"; line != want {
		t.Fatalf("line = %q, want relative-center fallback %q", line, want)
	}
}

func TestRenderPlaceholderWidthAlign(t *testing.T) {
	st := status.NewState()
	st.Resize(24, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("main")})
	st.UpdateModule(status.ModuleSnapshot{ID: "time", Value: status.Text("14:32")})

	r := New(layout.New(24), nil)
	line := r.Render(st, layout.ParseFormat("{git:^10}||{time:>8}")).Line

	if want := "   main            14:32"; line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
}

func TestRenderRowSeparatorMarker(t *testing.T) {
	st := status.NewState()
	st.Resize(30, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "env", Value: status.Text("dev")})
	st.UpdateModule(status.ModuleSnapshot{ID: "runtime", Value: status.Text("linux")})
	st.UpdateModule(status.ModuleSnapshot{ID: "shell", Value: status.Text("zsh")})

	r := New(layout.New(30), nil)
	line := r.RenderRow(st, layout.ParseFormat("{env} | {runtime} | {shell}"), ' ', " : ").Line

	if want := "dev : linux : zsh             "; line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
}

func TestRenderRowSeparatorMarkerCollapsesAroundEmptyModules(t *testing.T) {
	st := status.NewState()
	st.Resize(24, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "env", Value: status.Text("")})
	st.UpdateModule(status.ModuleSnapshot{ID: "runtime", Value: status.Text("linux")})
	st.UpdateModule(status.ModuleSnapshot{ID: "shell", Value: status.Text("zsh")})

	r := New(layout.New(24), nil)
	line := r.RenderRow(st, layout.ParseFormat("{env} | {runtime} | {shell}"), ' ', " : ").Line

	if want := "linux : zsh             "; line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
}

func TestRenderRowSeparatorMarkerIgnoresWhitespaceLiteralsForCollapse(t *testing.T) {
	st := status.NewState()
	st.Resize(24, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "env", Value: status.Text("dev")})
	st.UpdateModule(status.ModuleSnapshot{ID: "runtime", Value: status.Text("")})
	st.UpdateModule(status.ModuleSnapshot{ID: "shell", Value: status.Text("")})

	r := New(layout.New(24), nil)
	line := r.RenderRow(st, layout.ParseFormat("{env} | {runtime} | {shell} || {time}"), ' ', " : ").Line

	if strings.Contains(line, " : ") {
		t.Fatalf("line kept dangling separator next to whitespace literal: %q", line)
	}
	if !strings.HasPrefix(line, "dev") {
		t.Fatalf("line = %q, want env prefix", line)
	}
}

func TestRenderEscapedPipeLiteral(t *testing.T) {
	st := status.NewState()
	st.Resize(20, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "env", Value: status.Text("dev")})
	st.UpdateModule(status.ModuleSnapshot{ID: "runtime", Value: status.Text("linux")})

	r := New(layout.New(20), nil)
	line := r.RenderRow(st, layout.ParseFormat(`{env}\|{runtime}`), ' ', " : ").Line

	if want := "dev|linux           "; line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
}

// A border row ('-' fill) fills the whole width with dashes, draws edge caps, and
// places the block in its slot — exactly barWidth cells wide.
func TestRenderRowBorderFill(t *testing.T) {
	st := status.NewState()
	st.Resize(30, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("main")})

	r := New(layout.New(30), theme.Default(theme.NoColor))
	line := r.RenderRow(st, layout.ParseFormat("|| {git} ||"), '-', "").Line

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
	line := r.RenderRow(st, layout.ParseFormat("|| {git} ||"), '─', "").Line

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
	line := r.RenderRow(st, blocks, ' ', "").Line

	if line != strings.Repeat(" ", 20) {
		t.Fatalf("empty main-bar git block rendered %q, want blank bar", line)
	}
}

func TestRenderUsesModuleIDStyleFallback(t *testing.T) {
	st := status.NewState()
	st.Resize(20, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "time", Value: status.Text("12:34")})

	r := New(layout.New(20), theme.Default(theme.TrueColor))
	r.SetStyles(map[string]style.Style{
		// accent=brightcyan RGB{0,255,255}; muted=brightblack used as fg
		"time": {FG: "muted", BG: "accent", Bold: true, PaddingLeft: 1, PaddingRight: 1},
	})
	line := r.RenderRow(st, layout.ParseFormat("{time}"), ' ', "").Line

	// BG accent = brightcyan = RGB{0,255,255}
	if !strings.Contains(line, "\x1b[48;2;0;255;255m") {
		t.Fatalf("module-id style bg missing: %q", line)
	}
	if !strings.Contains(stripANSI(line), " 12:34 ") {
		t.Fatalf("module-id style padding missing: %q", line)
	}
}

func TestRenderAccountsForStyledRightAnchorWidth(t *testing.T) {
	st := status.NewState()
	st.Resize(20, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "time", Value: status.Text("12:34")})

	r := New(layout.New(20), theme.Default(theme.NoColor))
	r.SetStyles(map[string]style.Style{
		"time": {
			LeftCap:      " ",
			RightCap:     " ",
			PaddingLeft:  1,
			PaddingRight: 1,
		},
	})
	line := r.RenderRow(st, layout.ParseFormat("left||{time}"), ' ', "").Line

	if got := width.String(line); got != 20 {
		t.Fatalf("styled right anchor width = %d, want 20: %q", got, line)
	}
	if want := "left       " + "  12:34  "; line != want {
		t.Fatalf("styled right anchor = %q, want %q", line, want)
	}
}

func TestCommandGlintKeepsVisibleText(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.Shell.ActiveCommand = "npm test"
	st.AnimationPhase = 1
	st.ActiveCommandAnimating = true
	st.UpdateModule(status.ModuleSnapshot{ID: "command", Value: status.Text("npm test"), AnimationSuppressed: false})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetAnimations(map[string]Animation{"command": {Mode: "glint", Trigger: "active"}})
	line := r.RenderRow(st, layout.ParseFormat("{command}"), ' ', "").Line

	if !strings.Contains(stripANSI(line), "npm test") {
		t.Fatalf("command glint changed visible text: %q", line)
	}
	// command has no explicit FG, so glint falls back to static rendering — no truecolor codes.
}

func TestCommandDoneStopsGlint(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.Shell.LastCommand = "npm test"
	st.Shell.LastExitCode = 2
	st.Shell.LastDurationMS = 8420
	st.UpdateModule(status.ModuleSnapshot{ID: "command", Value: status.Text("npm test exit 2 8.4s"), AnimationSuppressed: true})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetAnimations(map[string]Animation{"command": {Mode: "glint", Trigger: "active"}})
	line := r.RenderRow(st, layout.ParseFormat("{command}"), ' ', "").Line

	if strings.Contains(line, "\x1b[38;2;") {
		t.Fatalf("done command should not use glint colors: %q", line)
	}
	if !strings.Contains(stripANSI(line), "npm test exit 2 8.4s") {
		t.Fatalf("done command missing visible text: %q", line)
	}
}

func TestCommandDoneSpansColorExitAndDimDuration(t *testing.T) {
	st := status.NewState()
	st.Resize(60, 1, false)
	st.UpdateModule(status.ModuleSnapshot{
		ID:    "command",
		Value: status.Text("sleep 5 • sigint • 1s"),
		Spans: []status.TextSpan{
			{Text: "sleep 5 • "},
			{Text: "sigint", Role: "exit", Level: status.LevelError},
			{Text: " • "},
			{Text: "1s", Role: "duration"},
		},
	})

	r := New(layout.New(60), theme.Default(theme.TrueColor))
	line := r.RenderRow(st, layout.ParseFormat("{command}"), ' ', "").Line
	if !strings.Contains(line, "\x1b[38;2;205;0;0m") {
		t.Fatalf("exit span should be error-colored: %q", line)
	}
	if !containsDimSGR(line) {
		t.Fatalf("duration span should be dimmed: %q", line)
	}
	if !strings.Contains(stripANSI(line), "sleep 5 • sigint • 1s") {
		t.Fatalf("styled command lost visible text: %q", line)
	}
}

func TestCommandDoneSpansWorkWithRightAlignment(t *testing.T) {
	// Blocks with explicit width and right alignment are padded by width.Pad before
	// applySnapshotSpans is called. The leading spaces must not break span lookup.
	st := status.NewState()
	st.Resize(60, 1, false)
	st.UpdateModule(status.ModuleSnapshot{
		ID:    "command",
		Value: status.Text("sleep 5 • sigint • 1s"),
		Spans: []status.TextSpan{
			{Text: "sleep 5 • "},
			{Text: "sigint", Role: "exit", Level: status.LevelError},
			{Text: " • "},
			{Text: "1s", Role: "duration"},
		},
	})

	r := New(layout.New(60), theme.Default(theme.TrueColor))
	// {command:40:right} pads the text with leading spaces for right-alignment.
	line := r.RenderRow(st, layout.ParseFormat("{command:40:right}"), ' ', "").Line
	if !strings.Contains(line, "\x1b[38;2;205;0;0m") {
		t.Fatalf("exit span should be error-colored in right-aligned block: %q", line)
	}
	if !containsDimSGR(line) {
		t.Fatalf("duration span should be dimmed in right-aligned block: %q", line)
	}
	if !strings.Contains(stripANSI(line), "sleep 5 • sigint • 1s") {
		t.Fatalf("styled command lost visible text in right-aligned block: %q", line)
	}
}

func TestCommandGlintStableWidthAndSinglePassCycle(t *testing.T) {
	render := func(phase int) string {
		st := status.NewState()
		st.Resize(40, 1, false)
		st.Shell.ActiveCommand = "sleep 30"
		st.AnimationPhase = phase
		st.ActiveCommandAnimating = true
		st.UpdateModule(status.ModuleSnapshot{ID: "command", Value: status.Text("sleep 30"), AnimationSuppressed: false})
		r := New(layout.New(40), theme.Default(theme.TrueColor))
		r.SetStyles(map[string]style.Style{"command": {FG: "#e5e5e5"}})
		r.SetAnimations(map[string]Animation{"command": {Mode: "glint", Trigger: "active"}})
		return r.RenderRow(st, layout.ParseFormat("{command}"), ' ', "").Line
	}
	// Only colors change between frames: the visible cells stay identical, so the
	// display width never shifts.
	base := stripANSI(render(0))
	for _, phase := range []int{1, 2, 3, 4, 7, 8, 11} {
		if got := stripANSI(render(phase)); got != base {
			t.Fatalf("phase %d changed visible cells: %q vs %q", phase, got, base)
		}
	}
	cycle := glintCycleLength(len("sleep 30"))
	if strings.Contains(render(0), "\x1b[38;2;229;229;229m") {
		t.Fatalf("first off-left frame should not show the highlight: %q", render(0))
	}
	if strings.Contains(render(cycle-1), "\x1b[38;2;229;229;229m") {
		t.Fatalf("last off-right frame should not show the next highlight: %q", render(cycle-1))
	}
	if !strings.Contains(render(cycle+1), "\x1b[38;2;110;110;110m") {
		t.Fatalf("next pass should start only after previous pass has disappeared: %q", render(cycle+1))
	}
}

func TestCommandGlintSuppressedPassFinishesBeforeDim(t *testing.T) {
	render := func(phase int) (string, bool) {
		st := status.NewState()
		st.Resize(40, 1, false)
		st.Shell.ActiveCommand = "sleep 30"
		st.AnimationPhase = phase
		st.UpdateModule(status.ModuleSnapshot{ID: "command", Value: status.Text("sleep 30"), AnimationSuppressed: true})
		r := New(layout.New(40), theme.Default(theme.TrueColor))
		r.SetStyles(map[string]style.Style{"command": {FG: "#e5e5e5"}})
		var active atomic.Bool
		r.SetChangeFlag(&active)
		r.SetAnimations(map[string]Animation{"command": {Mode: "glint", Trigger: "active"}})
		return r.RenderRow(st, layout.ParseFormat("{command}"), ' ', "").Line, active.Load()
	}

	line, active := render(glintHalfWidth)
	if !strings.Contains(line, "\x1b[38;2;229;229;229m") || !active {
		t.Fatalf("suppressed command should finish visible glint pass: active=%t line=%q", active, line)
	}
	line, active = render(glintCycleLength(len("sleep 30")))
	if strings.Contains(line, "\x1b[38;2;229;229;229m") || active {
		t.Fatalf("after glint exits, suppressed command should be dim: active=%t line=%q", active, line)
	}
	if !strings.Contains(line, "\x1b[38;2;96;96;96m") {
		t.Fatalf("suppressed command should settle to dim text: %q", line)
	}
}

func TestCommandGlintStopsWhenIdle(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.Shell.ActiveCommand = "codex"
	st.ActiveCommandAnimating = false
	st.UpdateModule(status.ModuleSnapshot{ID: "command", Value: status.Text("codex"), AnimationSuppressed: true})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetStyles(map[string]style.Style{"command": {FG: "#e5e5e5"}})
	r.SetAnimations(map[string]Animation{"command": {Mode: "glint", Trigger: "active"}})
	line := r.RenderRow(st, layout.ParseFormat("{command}"), ' ', "").Line

	if strings.Contains(line, "\x1b[38;2;229;229;229m") {
		t.Fatalf("idle command should not show bright glint highlight: %q", line)
	}
	if !strings.Contains(line, "\x1b[38;2;96;96;96m") {
		t.Fatalf("idle command should stay dimmed: %q", line)
	}
	if !strings.Contains(stripANSI(line), "codex") {
		t.Fatalf("idle command missing visible text: %q", line)
	}
}

func TestAnyModuleCanGlint(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.AnimationPhase = 1
	st.UpdateModule(status.ModuleSnapshot{ID: "time", Value: status.Text("12:34")})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetAnimations(map[string]Animation{"time": {Mode: "glint", Trigger: "active"}})
	line := r.RenderRow(st, layout.ParseFormat("{time}"), ' ', "").Line

	// time has no explicit FG, so glint falls back to static rendering — no truecolor codes.
	if !strings.Contains(stripANSI(line), "12:34") {
		t.Fatalf("animated time block changed visible text: %q", line)
	}
}

func TestGlintHighlightTracksForegroundHue(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.AnimationPhase = glintHalfWidth
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("main-branch")})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetStyles(map[string]style.Style{"git": {FG: "#00cd00"}})
	r.SetAnimations(map[string]Animation{"git": {Mode: "glint", Trigger: "active"}})
	line := r.RenderRow(st, layout.ParseFormat("{git}"), ' ', "").Line

	if !strings.Contains(line, "\x1b[38;2;0;205;0m") {
		t.Fatalf("glint highlight should use the configured green fg: %q", line)
	}
	if !strings.Contains(line, "\x1b[38;2;0;86;0m") {
		t.Fatalf("glint base should be a dimmed green fg: %q", line)
	}
	if strings.Contains(line, "\x1b[38;2;255;240;194m") {
		t.Fatalf("glint should not use the old fixed warm highlight: %q", line)
	}
}

func TestPulseKeepsVisibleTextAndChangesColors(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.AnimationPhase = 4
	st.UpdateModule(status.ModuleSnapshot{ID: "hostname", Value: status.Text("myhost")})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetAnimations(map[string]Animation{"hostname": {Mode: "pulse", Trigger: "active"}})
	line := r.RenderRow(st, layout.ParseFormat("{hostname}"), ' ', "").Line

	if !strings.Contains(stripANSI(line), "myhost") {
		t.Fatalf("pulse changed visible text: %q", line)
	}
	// pulse emits a truecolor FG escape
	if !strings.Contains(line, "\x1b[38;2;") {
		t.Fatalf("pulse missing truecolor FG escape: %q", line)
	}
}

func TestPulseStableDisplayWidth(t *testing.T) {
	render := func(phase int) string {
		st := status.NewState()
		st.Resize(40, 1, false)
		st.AnimationPhase = phase
		st.UpdateModule(status.ModuleSnapshot{ID: "hostname", Value: status.Text("myhost")})
		r := New(layout.New(40), theme.Default(theme.TrueColor))
		r.SetAnimations(map[string]Animation{"hostname": {Mode: "pulse", Trigger: "active"}})
		return r.RenderRow(st, layout.ParseFormat("{hostname}"), ' ', "").Line
	}
	base := width.String(stripANSI(render(0)))
	for _, phase := range []int{1, 4, 8, 12, 15} {
		if got := width.String(stripANSI(render(phase))); got != base {
			t.Fatalf("phase %d changed display width: %d vs %d", phase, got, base)
		}
	}
}

func TestBlinkKeepsVisibleTextAndAlternates(t *testing.T) {
	render := func(phase int) string {
		st := status.NewState()
		st.Resize(40, 1, false)
		st.AnimationPhase = phase
		st.UpdateModule(status.ModuleSnapshot{ID: "hostname", Value: status.Text("myhost")})
		r := New(layout.New(40), theme.Default(theme.TrueColor))
		r.SetAnimations(map[string]Animation{"hostname": {Mode: "blink", Trigger: "active"}})
		return r.RenderRow(st, layout.ParseFormat("{hostname}"), ' ', "").Line
	}
	// Visible text must not change.
	for _, phase := range []int{0, blinkPeriod, blinkPeriod * 2} {
		if !strings.Contains(stripANSI(render(phase)), "myhost") {
			t.Fatalf("blink phase %d lost visible text", phase)
		}
	}
	// Odd half-cycles carry SGR Dim (code "2"), even half-cycles do not.
	dimLine := render(blinkPeriod)
	brightLine := render(0)
	if !containsDimSGR(dimLine) {
		t.Fatalf("blink dim phase missing SGR 2: %q", dimLine)
	}
	if containsDimSGR(brightLine) {
		t.Fatalf("blink bright phase should not carry SGR 2: %q", brightLine)
	}
}

func TestChangeAnimationStartsOnValueChange(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.AnimationPhase = 1
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("main")})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetAnimations(map[string]Animation{"git": {Mode: "blink", Trigger: "change", DurationTicks: 4}})
	first := r.RenderRow(st, layout.ParseFormat("{git}"), ' ', "").Line
	if strings.Contains(first, "\x1b[2m") {
		t.Fatalf("first observed value should not animate: %q", first)
	}

	st.AnimationPhase = blinkPeriod
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("feature")})
	changed := r.RenderRow(st, layout.ParseFormat("{git}"), ' ', "").Line
	if !containsDimSGR(changed) {
		t.Fatalf("changed value should blink: %q", changed)
	}
}

func TestTimeIgnoresChangeAnimation(t *testing.T) {
	st := status.NewState()
	st.Resize(40, 1, false)
	st.AnimationPhase = blinkPeriod
	st.UpdateModule(status.ModuleSnapshot{ID: "time", Value: status.Text("12:34")})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetAnimations(map[string]Animation{"time": {Mode: "blink", Trigger: "change", DurationTicks: 4}})
	line := r.RenderRow(st, layout.ParseFormat("{time}"), ' ', "").Line
	if strings.Contains(line, "\x1b[2m") {
		t.Fatalf("time change animation should be ignored: %q", line)
	}
}

func TestPartialStyleOverrideClearsBoldDefault(t *testing.T) {
	// defaultStyleFor sets Bold=true for "git". A user who writes only
	// [style.git] fg = "white" must not inherit Bold — mergeStyle must not
	// OR booleans; the overlay value wins.
	st := status.NewState()
	st.Resize(40, 1, false)
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("main")})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetStyles(map[string]style.Style{"git": {FG: "white"}})
	line := r.RenderRow(st, layout.ParseFormat("{git}"), ' ', "").Line
	if strings.Contains(line, "\x1b[1m") {
		t.Fatalf("partial style override should clear Bold default for git: %q", line)
	}
	if !strings.Contains(stripANSI(line), "main") {
		t.Fatalf("git block lost visible text: %q", line)
	}
}

func TestMultiRowChangeFlagAccumulates(t *testing.T) {
	// Row 0: suppressed command with glint still mid-pass (frameActiveAnimation=true).
	// Row 1: time with no animation. changeFlag must stay true after both rows —
	// the second RenderRow must not overwrite row 0's result.
	st := status.NewState()
	st.Resize(40, 1, false)
	st.Shell.ActiveCommand = "sleep 30"
	st.AnimationPhase = glintHalfWidth // glint is mid-pass (center=0, visible)
	st.UpdateModule(status.ModuleSnapshot{ID: "command", Value: status.Text("sleep 30"), AnimationSuppressed: true})
	st.UpdateModule(status.ModuleSnapshot{ID: "time", Value: status.Text("12:34")})

	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetStyles(map[string]style.Style{"command": {FG: "#e5e5e5"}})
	r.SetAnimations(map[string]Animation{"command": {Mode: "glint", Trigger: "active"}})
	var flag atomic.Bool
	r.SetChangeFlag(&flag)

	r.RenderRow(st, layout.ParseFormat("{command}"), ' ', "")
	r.RenderRow(st, layout.ParseFormat("{time}"), ' ', "")
	if !flag.Load() {
		t.Fatal("changeFlag should remain true after rendering a second empty-animation row")
	}
}

func TestChangeAnimationExpiresOnPhaseReset(t *testing.T) {
	// Simulate a value change at phase=150, then a phase reset to 0.
	// The change animation must expire immediately on the reset, not stall for ~19s.
	r := New(layout.New(40), theme.Default(theme.TrueColor))
	r.SetAnimations(map[string]Animation{"git": {Mode: "blink", Trigger: "change", DurationTicks: 8}})

	st := status.NewState()
	st.Resize(40, 1, false)
	st.AnimationPhase = blinkPeriod
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("main")})
	r.RenderRow(st, layout.ParseFormat("{git}"), ' ', "")

	st.AnimationPhase = 150
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("feature")})
	r.RenderRow(st, layout.ParseFormat("{git}"), ' ', "")

	// Phase reset to 0 (as happens when command ends).
	st.AnimationPhase = 0
	st.UpdateModule(status.ModuleSnapshot{ID: "git", Value: status.Text("feature")})
	line := r.RenderRow(st, layout.ParseFormat("{git}"), ' ', "").Line
	if containsDimSGR(line) {
		t.Fatalf("change animation should expire on phase reset, not stall: %q", line)
	}
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func containsDimSGR(s string) bool {
	return strings.Contains(s, "\x1b[2m") || strings.Contains(s, ";2m")
}

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

// --- Template module tests ---

func newStateWithModules(cols int, kvs map[string]string) status.StatusState {
	st := status.NewState()
	st.Resize(uint16(cols), 1, false)
	for id, val := range kvs {
		st.UpdateModule(status.ModuleSnapshot{ID: status.ModuleID(id), Value: status.Text(val)})
	}
	return st
}

func TestTemplateBasicExpand(t *testing.T) {
	st := newStateWithModules(80, map[string]string{"user": "alice", "hostname": "myhost"})
	r := New(layout.New(80), nil)
	r.SetTemplates(map[string]TemplateSpec{
		"identity": {Blocks: layout.ParseFormat("{user}@{hostname}")},
	})
	out := stripANSI(r.Render(st, layout.ParseFormat("{identity}")).Line)
	if !strings.Contains(out, "alice@myhost") {
		t.Fatalf("template expand = %q, want alice@myhost", out)
	}
}

func TestTemplateHideWhenEmptyAllEmpty(t *testing.T) {
	st := newStateWithModules(80, map[string]string{"user": "", "hostname": ""})
	r := New(layout.New(80), nil)
	r.SetTemplates(map[string]TemplateSpec{
		"identity": {
			Blocks:        layout.ParseFormat("{user}@{hostname}"),
			HideWhenEmpty: true,
		},
	})
	out := stripANSI(r.Render(st, layout.ParseFormat("{identity}")).Line)
	if strings.Contains(out, "@") {
		t.Fatalf("should be hidden when all modules empty, got %q", out)
	}
}

func TestTemplateHideWhenEmptyOneNonEmpty(t *testing.T) {
	st := newStateWithModules(80, map[string]string{"user": "alice", "hostname": ""})
	r := New(layout.New(80), nil)
	r.SetTemplates(map[string]TemplateSpec{
		"identity": {
			Blocks:        layout.ParseFormat("{user}@{hostname}"),
			HideWhenEmpty: true,
		},
	})
	out := stripANSI(r.Render(st, layout.ParseFormat("{identity}")).Line)
	if !strings.Contains(out, "alice") {
		t.Fatalf("should show when at least one module non-empty, got %q", out)
	}
	if !strings.Contains(out, "@") {
		t.Fatalf("literal should be kept when at least one module non-empty, got %q", out)
	}
}

func TestTemplateCollapseWhitespace(t *testing.T) {
	st := newStateWithModules(80, map[string]string{"a": "foo", "b": "bar"})
	r := New(layout.New(80), nil)
	r.SetTemplates(map[string]TemplateSpec{
		"combo": {
			Blocks:             layout.ParseFormat("{a}  {b}"),
			CollapseWhitespace: true,
		},
	})
	out := strings.TrimSpace(stripANSI(r.Render(st, layout.ParseFormat("{combo}")).Line))
	if strings.Contains(out, "  ") {
		t.Fatalf("double space not collapsed: %q", out)
	}
	if out != "foo bar" {
		t.Fatalf("collapse result = %q, want %q", out, "foo bar")
	}
}

func TestTemplateConditionalSeparators(t *testing.T) {
	st := newStateWithModules(80, map[string]string{"a": "foo", "b": "", "c": "bar"})
	r := New(layout.New(80), nil)
	r.SetTemplates(map[string]TemplateSpec{
		"combo": {
			Blocks:    layout.ParseFormat("{a} | {b} | {c}"),
			Separator: "•",
		},
	})
	out := strings.TrimSpace(stripANSI(r.Render(st, layout.ParseFormat("{combo}")).Line))
	if out != "foo • bar" {
		t.Fatalf("template conditional separators = %q, want foo • bar", out)
	}
}

func TestTemplateMaxWidth(t *testing.T) {
	st := newStateWithModules(80, map[string]string{"msg": "hello world"})
	r := New(layout.New(80), nil)
	r.SetTemplates(map[string]TemplateSpec{
		"short": {Blocks: layout.ParseFormat("{msg}"), MaxWidth: 5},
	})
	out := stripANSI(r.Render(st, layout.ParseFormat("{short}")).Line)
	trimmed := strings.TrimSpace(out)
	if width.String(trimmed) > 5 {
		t.Fatalf("template max_width not applied: %q", trimmed)
	}
}
