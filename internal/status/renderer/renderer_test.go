package renderer

import (
	"strings"
	"testing"

	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/layout"
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
