// Package renderer turns a prepared StatusState into a single bar line. It reads
// state only — never queries providers. The Render result also carries click
// zones, even though mouse handling is post-MVP, so adding mouse support later
// needs no renderer rewrite (spec §8.6, arch.md §15).
package renderer

import (
	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/layout"
)

// Action is what a click on a zone triggers (post-MVP; ignored when mouse is off).
type Action struct {
	Kind  string // open_url | run_command | show_agent_details | ...
	Param string
}

// ClickZone maps a cell range on the bar row to an Action (arch.md §15).
type ClickZone struct {
	StartCol uint16
	EndCol   uint16
	Action   Action
}

// RenderedBar is the renderer output: the styled line plus optional click zones.
type RenderedBar struct {
	Line       string
	ClickZones []ClickZone
}

// Renderer draws the bar. It holds the layout engine and styling context but no
// data sources.
type Renderer struct {
	engine *layout.Engine
}

// New creates a renderer over a layout engine.
func New(engine *layout.Engine) *Renderer {
	return &Renderer{engine: engine}
}

// Render produces the bar line for the given state and blocks.
//
// IMPORTANT (spec §8.6, docs/terminal-safety.md): the caller draws this with
// absolute positioning — save cursor → move to bar row → clear line → write
// Line → reset → restore cursor — and NEVER appends a newline.
//
// TODO scaffold (plan 09): arrange blocks, render each module value via its
// style/theme, measure with status/width, and assemble the line + click zones.
func (r *Renderer) Render(st status.StatusState, blocks []layout.Block) RenderedBar {
	_ = r.engine.Arrange(blocks)
	_ = st
	return RenderedBar{}
}
