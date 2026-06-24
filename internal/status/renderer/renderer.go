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

// Renderer draws the bar. It holds the layout engine and styling context but no
// data sources.
type Renderer struct {
	engine *layout.Engine
}

const redStatusBar = "\x1b[37;41m"

// New creates a renderer over a layout engine.
func New(engine *layout.Engine) *Renderer {
	return &Renderer{engine: engine}
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
	lastStyle := map[layout.Anchor]string{}
	for i, placement := range placements {
		if !placement.Visible {
			continue
		}
		text := width.Truncate(values[i], placement.Width, placement.Block.Truncate)
		text = width.Pad(text, placement.Width, string(placement.Block.Align))
		plainSections[placement.Block.Anchor] += text
		style := blockStyle(placement.Block)
		if placement.Block.IsLiteral() {
			if previous, ok := lastStyle[placement.Block.Anchor]; ok {
				style = previous
			}
		} else {
			lastStyle[placement.Block.Anchor] = style
		}
		sections[placement.Block.Anchor] += style + text + redStatusBar
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
	return RenderedBar{Line: redStatusBar + line}
}

func blockStyle(block layout.Block) string {
	if block.IsLiteral() {
		return redStatusBar
	}
	color := "\x1b[37;41m"
	switch block.ModuleID {
	case "hostname":
		color = "\x1b[37;44m"
	case "cwd":
		color = "\x1b[30;43m"
	case "time":
		color = "\x1b[1;30;42m"
	}
	return color
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
