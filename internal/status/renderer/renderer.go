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
	// animations enables visual effects by module ID. Effects are applied after
	// layout so ANSI bytes never affect display-width calculations.
	animations map[string]Animation
	// base is the SGR establishing the bar's base colors, re-emitted after every
	// styled segment so the background spans the whole row. Empty in no-color mode.
	base string
}

// Animation is a resolved per-module animation setting.
type Animation struct {
	Mode string
}

// New creates a renderer over a layout engine and theme. A nil theme renders
// plain (no-color) output.
func New(engine *layout.Engine, th *theme.Theme) *Renderer {
	r := &Renderer{engine: engine, theme: th, styles: map[string]style.Style{}, animations: map[string]Animation{}}
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

// SetAnimations installs config-resolved per-module animations.
func (r *Renderer) SetAnimations(animations map[string]Animation) {
	if animations != nil {
		r.animations = animations
	}
}

// Render produces the bar line for the given state and blocks, space-filled.
//
// IMPORTANT (spec §8.6, docs/terminal-safety.md): the caller draws this with
// absolute positioning — save cursor → move to bar row → clear line → write
// Line → reset → restore cursor — and NEVER appends a newline.
func (r *Renderer) Render(st status.StatusState, blocks []layout.Block) RenderedBar {
	return r.RenderRow(st, blocks, ' ')
}

// RenderRow renders one bar row with a chosen fill character used for the gaps
// between the left/center/right slots and the edge caps. A space fill yields a
// plain bar; a '-' fill yields a "border" row like
// `--{left} ----- {center} ----- {right} --` (multi-line panels, the top line).
// Only the three anchor slots exist; blocks cannot be added beyond them.
func (r *Renderer) RenderRow(st status.StatusState, blocks []layout.Block, fill rune) RenderedBar {
	fillStr := string(fill)
	border := fill != ' '

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
		if !placement.Block.IsLiteral() && values[i] == "" {
			continue
		}
		text := width.Truncate(values[i], placement.Width, placement.Block.Truncate)
		text = width.Pad(text, placement.Width, string(placement.Block.Align))
		plainSections[placement.Block.Anchor] += text
		// Each segment is styled and reset, then the base colors are re-emitted so
		// the bar background continues across gaps and padding.
		blockStyle := r.styleFor(placement.Block)
		if r.shouldGlint(st, placement.Block) {
			sections[placement.Block.Anchor] += r.applyGlint(text, blockStyle, st.AnimationPhase) + r.base
			continue
		}
		sections[placement.Block.Anchor] += blockStyle.Apply(text, r.theme) + r.base
	}

	// Reserve two cells on each edge for caps when bordered; the inner layout is
	// computed against the remaining width.
	caps := ""
	target := r.engine.BarWidth()
	if border {
		caps = strings.Repeat(fillStr, 2)
		target = max(0, target-2*width.String(caps))
	}

	left := sections[layout.AnchorLeft]
	center := sections[layout.AnchorCenter]
	right := sections[layout.AnchorRight]
	plainLeft := plainSections[layout.AnchorLeft]
	plainCenter := plainSections[layout.AnchorCenter]
	plainRight := plainSections[layout.AnchorRight]
	if border {
		left, plainLeft = emptyWhitespaceSection(left, plainLeft)
		center, plainCenter = emptyWhitespaceSection(center, plainCenter)
		right, plainRight = emptyWhitespaceSection(right, plainRight)
	}
	line := left
	plainLine := plainLeft
	if plainCenter != "" {
		gap := max(0, (target-width.String(plainLeft)-width.String(plainRight)-width.String(plainCenter))/2)
		line += strings.Repeat(fillStr, gap) + center
		plainLine += strings.Repeat(fillStr, gap) + plainCenter
	}
	gap := max(0, target-width.String(plainLine)-width.String(plainRight))
	line += strings.Repeat(fillStr, gap) + right
	plainLine += strings.Repeat(fillStr, gap) + plainRight
	// ANSI styles are deliberately applied after layout. All spacing decisions use
	// the unstyled twin line, otherwise escape bytes would corrupt cell widths.
	if width.String(plainLine) < target {
		line += strings.Repeat(fillStr, target-width.String(plainLine))
	}
	return RenderedBar{Line: r.base + caps + line + caps}
}

func emptyWhitespaceSection(styled, plain string) (string, string) {
	if strings.TrimSpace(plain) == "" {
		return "", ""
	}
	return styled, plain
}

// styleFor resolves the style for a block: an explicit config style by StyleID,
// otherwise a readable default that colors a few well-known modules via theme
// tokens (never raw ANSI — arch.md §16). All defaults share the base background
// so the bar reads as one band.
func (r *Renderer) styleFor(block layout.Block) style.Style {
	moduleID := canonicalModuleID(block.ModuleID)
	if block.StyleID != "" {
		if s, ok := r.styles[block.StyleID]; ok {
			return s
		}
	}
	s := style.Style{FG: "base.fg", BG: "base.bg"}
	if block.IsLiteral() {
		return s
	}
	switch moduleID {
	case "hostname":
		s.FG, s.Bold = "accent", true
	case "time":
		s.FG = "muted"
	case "cwd":
		s.FG = "base.fg"
	case "git":
		s.FG, s.Bold = "ok", true
	case "active_command":
		s.FG = "#f2b35d"
	}
	return s
}

func (r *Renderer) shouldGlint(st status.StatusState, block layout.Block) bool {
	if block.IsLiteral() || r.theme == nil || r.theme.Mode() == theme.NoColor {
		return false
	}
	moduleID := canonicalModuleID(block.ModuleID)
	animation, ok := r.animations[moduleID]
	if !ok || animation.Mode != "glint" {
		return false
	}
	if moduleID == "active_command" {
		return st.Shell.ActiveCommand != "" && st.ActiveCommandAnimating
	}
	return true
}

func (r *Renderer) applyGlint(content string, s style.Style, phase int) string {
	body := strings.Repeat(" ", max(0, s.PaddingLeft)) + content + strings.Repeat(" ", max(0, s.PaddingRight))
	if r.theme == nil || r.theme.Mode() == theme.NoColor {
		return s.LeftSeparator + body + s.RightSeparator
	}
	runes := []rune(body)
	if len(runes) == 0 {
		return s.Apply(content, r.theme)
	}
	if phase < 0 {
		phase = -phase
	}
	center := phase % (len(runes) + 4)
	var b strings.Builder
	b.WriteString(s.LeftSeparator)
	b.WriteString(r.theme.BG(s.BG))
	b.WriteString(styleAttrs(s))
	for i, ch := range runes {
		switch abs(i - center) {
		case 0:
			b.WriteString(r.theme.FG("#fff0c2"))
		case 1:
			b.WriteString(r.theme.FG("#ffd58a"))
		default:
			b.WriteString(r.theme.FG(s.FG))
		}
		b.WriteRune(ch)
	}
	b.WriteString(theme.Reset)
	b.WriteString(s.RightSeparator)
	return b.String()
}

func styleAttrs(s style.Style) string {
	var codes []string
	if s.Bold {
		codes = append(codes, "1")
	}
	if s.Dim {
		codes = append(codes, "2")
	}
	if s.Italic {
		codes = append(codes, "3")
	}
	if s.Underline {
		codes = append(codes, "4")
	}
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

func blockValue(st status.StatusState, block layout.Block) string {
	if block.IsLiteral() {
		return block.Text
	}
	snapshot, ok := st.Modules[status.ModuleID(canonicalModuleID(block.ModuleID))]
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

func canonicalModuleID(id string) string {
	if id == "cmd" {
		return "active_command"
	}
	return id
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
