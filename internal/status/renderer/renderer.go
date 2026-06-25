// Package renderer turns a prepared StatusState into a single bar line. It reads
// state only — never queries providers. The Render result also carries click
// zones, even though mouse handling is post-MVP, so adding mouse support later
// needs no renderer rewrite (spec §8.6, arch.md §15).
package renderer

import (
	"fmt"
	"strings"

	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/layout"
	"github.com/hsgiga/ptyline/internal/status/style"
	"github.com/hsgiga/ptyline/internal/status/theme"
	"github.com/hsgiga/ptyline/internal/status/width"
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

// Renderer draws the bar. It holds the layout engine and styling context (theme
// + per-block styles) but no data sources.
type Renderer struct {
	engine *layout.Engine
	theme  *theme.Theme
	// styles overrides the default block style by StyleID (from config). Empty in
	// the MVP format-string path; populated when structured [[bar.block]] styles
	// are wired.
	styles map[string]style.Style
	// base is the SGR establishing the bar's base colors, re-emitted after every
	// styled segment so the background spans the whole row. Empty in no-color mode.
	base string
}

// New creates a renderer over a layout engine and theme. A nil theme renders
// plain (no-color) output.
func New(engine *layout.Engine, th *theme.Theme) *Renderer {
	r := &Renderer{engine: engine, theme: th, styles: map[string]style.Style{}}
	if th != nil && th.Mode() != theme.NoColor {
		r.base = th.FG("base.fg") + th.BG("base.bg")
	}
	return r
}

// SetStyles installs config-resolved per-StyleID styles (post-MVP wiring).
func (r *Renderer) SetStyles(styles map[string]style.Style) {
	if styles != nil {
		r.styles = styles
	}
}

// Render produces the bar line for the given state and blocks.
//
// IMPORTANT (spec §8.6, docs/terminal-safety.md): the caller draws this with
// absolute positioning — save cursor → move to bar row → clear line → write
// Line → reset → restore cursor — and NEVER appends a newline.
func (r *Renderer) Render(st status.StatusState, blocks []layout.Block) RenderedBar {
	natural := make([]int, len(blocks))
	values := make([]string, len(blocks))
	for i, block := range blocks {
		values[i] = blockValue(st, block)
		natural[i] = width.String(values[i])
	}
	placements := r.engine.Arrange(blocks, natural)
	sections := map[layout.Anchor]string{}
	plainSections := map[layout.Anchor]string{}
	for i, placement := range placements {
		if !placement.Visible {
			continue
		}
		text := width.Truncate(values[i], placement.Width, placement.Block.Truncate)
		text = width.Pad(text, placement.Width, string(placement.Block.Align))
		plainSections[placement.Block.Anchor] += text
		// Each segment is styled and reset, then the base colors are re-emitted so
		// the bar background continues across gaps and padding.
		sections[placement.Block.Anchor] += r.styleFor(placement.Block).Apply(text, r.theme) + r.base
	}
	left := sections[layout.AnchorLeft]
	center := sections[layout.AnchorCenter]
	right := sections[layout.AnchorRight]
	plainLeft := plainSections[layout.AnchorLeft]
	plainCenter := plainSections[layout.AnchorCenter]
	plainRight := plainSections[layout.AnchorRight]
	line := left
	plainLine := plainLeft
	if plainCenter != "" {
		gap := max(0, (r.engine.BarWidth()-width.String(plainLeft)-width.String(plainRight)-width.String(plainCenter))/2)
		line += strings.Repeat(" ", gap) + center
		plainLine += strings.Repeat(" ", gap) + plainCenter
	}
	gap := max(0, r.engine.BarWidth()-width.String(plainLine)-width.String(plainRight))
	line += strings.Repeat(" ", gap) + right
	plainLine += strings.Repeat(" ", gap) + plainRight
	// ANSI styles are deliberately applied after layout. All spacing decisions use
	// the unstyled twin line, otherwise escape bytes would corrupt cell widths.
	if width.String(plainLine) < r.engine.BarWidth() {
		line += strings.Repeat(" ", r.engine.BarWidth()-width.String(plainLine))
	}
	return RenderedBar{Line: r.base + line}
}

// styleFor resolves the style for a block: an explicit config style by StyleID,
// otherwise a readable default that colors a few well-known modules via theme
// tokens (never raw ANSI — arch.md §16). All defaults share the base background
// so the bar reads as one band.
func (r *Renderer) styleFor(block layout.Block) style.Style {
	if block.StyleID != "" {
		if s, ok := r.styles[block.StyleID]; ok {
			return s
		}
	}
	s := style.Style{FG: "base.fg", BG: "base.bg"}
	if block.IsLiteral() {
		return s
	}
	switch block.ModuleID {
	case "hostname":
		s.FG, s.Bold = "accent", true
	case "time":
		s.FG = "muted"
	case "cwd":
		s.FG = "base.fg"
	}
	return s
}

func blockValue(st status.StatusState, block layout.Block) string {
	if block.IsLiteral() {
		return block.Text
	}
	snapshot, ok := st.Modules[status.ModuleID(block.ModuleID)]
	if !ok || snapshot.Err != nil {
		return ""
	}
	switch snapshot.Value.Kind {
	case status.KindText:
		return snapshot.Value.Text
	case status.KindNumber:
		return fmt.Sprint(snapshot.Value.Number)
	case status.KindBool:
		return fmt.Sprint(snapshot.Value.Bool)
	case status.KindStatus:
		if snapshot.Value.Status != nil {
			return snapshot.Value.Status.Text
		}
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
