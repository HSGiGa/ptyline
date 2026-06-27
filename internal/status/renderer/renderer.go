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
	// templates holds pre-parsed template module specs resolved at render time
	// from cached snapshots (no provider calls, no goroutines).
	templates map[string]TemplateSpec
	// icons holds config-resolved module icons. Icons are applied to non-empty
	// module values at render time so providers keep returning clean values.
	icons map[string]ModuleIcon
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
	r := &Renderer{engine: engine, theme: th, styles: map[string]style.Style{}, animations: map[string]Animation{}, templates: map[string]TemplateSpec{}, icons: map[string]ModuleIcon{}}
	if th != nil && th.Mode() != theme.NoColor {
		r.base = th.FG("base.fg") + th.BG("base.bg")
	}
	return r
}

// ModuleIcon is a display-only icon attached to a rendered module block.
type ModuleIcon struct {
	Position string // left | right
	Text     string
}

// SetStyles installs config-resolved per-StyleID styles (post-MVP wiring).
func (r *Renderer) SetStyles(styles map[string]style.Style) {
	if styles != nil {
		r.styles = styles
	}
}

// SetTemplates installs pre-parsed template module specs. Called once after
// config load; template values are resolved from cached snapshots at render time.
func (r *Renderer) SetTemplates(templates map[string]TemplateSpec) {
	if templates != nil {
		r.templates = templates
	}
}

// SetIcons installs config-resolved per-module icon settings.
func (r *Renderer) SetIcons(icons map[string]ModuleIcon) {
	if icons != nil {
		r.icons = icons
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

	// Compute the effective inner width before arranging so block widths are
	// allocated against the content area only (border caps are excluded).
	caps := ""
	target := r.engine.BarWidth()
	if border {
		caps = strings.Repeat(fillStr, 2)
		target = max(0, target-2*width.String(caps))
	}

	natural := make([]int, len(blocks))
	values := make([]string, len(blocks))
	styles := make([]style.Style, len(blocks))
	for i, block := range blocks {
		values[i] = blockValue(st, block, r.templates)
		if !block.IsLiteral() && values[i] != "" {
			values[i] = r.applyIcon(canonicalModuleID(block.ModuleID), values[i])
		}
		styles[i] = r.styleFor(block)
		if !block.IsLiteral() && values[i] == "" {
			continue
		}
		natural[i] = width.String(values[i]) + styles[i].OuterWidth()
	}
	placements := r.engine.ArrangeIn(blocks, natural, target)
	// Use per-anchor builders to avoid map allocations and string += copies.
	var styledL, styledC, styledR strings.Builder
	var plainL, plainC, plainR strings.Builder
	for i, placement := range placements {
		if !placement.Visible {
			continue
		}
		if !placement.Block.IsLiteral() && values[i] == "" {
			continue
		}
		blockStyle := styles[i]
		contentWidth := max(0, placement.Width-blockStyle.OuterWidth())
		text := width.Truncate(values[i], contentWidth, placement.Block.Truncate)
		text = width.Pad(text, contentWidth, string(placement.Block.Align))
		var sb, pb *strings.Builder
		switch placement.Block.Anchor {
		case layout.AnchorCenter:
			sb, pb = &styledC, &plainC
		case layout.AnchorRight:
			sb, pb = &styledR, &plainR
		default:
			sb, pb = &styledL, &plainL
		}
		pb.WriteString(blockStyle.Plain(text))
		// Each segment is styled and reset, then the base colors are re-emitted so
		// the bar background continues across gaps and padding.
		switch r.animationMode(st, placement.Block) {
		case AnimGlint:
			sb.WriteString(r.applyGlint(text, blockStyle, st.AnimationPhase))
			sb.WriteString(r.base)
		case AnimPulse:
			sb.WriteString(r.applyPulse(text, blockStyle, st.AnimationPhase))
			sb.WriteString(r.base)
		case AnimBlink:
			sb.WriteString(r.applyBlink(text, blockStyle, st.AnimationPhase))
			sb.WriteString(r.base)
		default:
			sb.WriteString(blockStyle.Apply(text, r.theme))
			sb.WriteString(r.base)
		}
	}

	left := styledL.String()
	center := styledC.String()
	right := styledR.String()
	plainLeft := plainL.String()
	plainCenter := plainC.String()
	plainRight := plainR.String()
	if border {
		left, plainLeft = emptyWhitespaceSection(left, plainLeft)
		center, plainCenter = emptyWhitespaceSection(center, plainCenter)
		right, plainRight = emptyWhitespaceSection(right, plainRight)
	}
	// Cache display widths to avoid repeating runewidth scans on the same string.
	wLeft := width.String(plainLeft)
	wCenter := width.String(plainCenter)
	wRight := width.String(plainRight)
	line := left
	plainLine := plainLeft
	wLine := wLeft
	if plainCenter != "" {
		gap := max(0, (target-wLeft-wRight-wCenter)/2)
		line += strings.Repeat(fillStr, gap) + center
		plainLine += strings.Repeat(fillStr, gap) + plainCenter
		wLine += gap + wCenter
	}
	gap := max(0, target-wLine-wRight)
	line += strings.Repeat(fillStr, gap) + right
	wLine += gap + wRight
	// ANSI styles are deliberately applied after layout. All spacing decisions use
	// the unstyled twin line, otherwise escape bytes would corrupt cell widths.
	if wLine < target {
		line += strings.Repeat(fillStr, target-wLine)
	}
	return RenderedBar{Line: r.base + caps + line + caps}
}

func (r *Renderer) applyIcon(moduleID, value string) string {
	icon, ok := r.icons[moduleID]
	if !ok || icon.Text == "" {
		return value
	}
	switch icon.Position {
	case "left":
		return icon.Text + " " + value
	case "right":
		return value + " " + icon.Text
	default:
		return value
	}
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
	if moduleID != "" {
		if s, ok := r.styles[moduleID]; ok {
			return s
		}
	}
	s := style.Style{} // no explicit fg/bg: terminal defaults
	if block.IsLiteral() {
		s.FG = "muted" // separators and frame chrome in bright black (8)
		return s
	}
	switch moduleID {
	case "hostname":
		s.FG, s.Bold = "ok", true // brightgreen 10 — matches user@host convention in bash/zsh/fish
	case "cwd":
		s.FG, s.Bold = "blue", true // 4 bold — matches bash \w (\e[01;34m)
	case "time":
		s.FG = "warn" // brightyellow 11
	case "git":
		s.FG, s.Bold = "ok", true // brightgreen 10; error/warn states are post-MVP
	case "command":
		// terminal default fg — command text blends with the frame line
	case "exit_code":
		s.FG = "error" // brightred 9
	case "ssh":
		s.FG, s.Bold = "warn", true
	}
	return s
}

// animationMode returns the animation mode for a block, or "" if no animation
// should run. Checks: color support, config opt-in, and the snapshot's
// AnimationSuppressed flag (set by modules that control their own animation
// timing, e.g. command only animates while a command is running).
func (r *Renderer) animationMode(st status.StatusState, block layout.Block) string {
	if block.IsLiteral() || r.theme == nil || r.theme.Mode() == theme.NoColor {
		return ""
	}
	anim, ok := r.animations[canonicalModuleID(block.ModuleID)]
	if !ok || anim.Mode == "" || anim.Mode == "none" {
		return ""
	}
	snap, hasSnap := st.Modules[status.ModuleID(canonicalModuleID(block.ModuleID))]
	if hasSnap && snap.AnimationSuppressed {
		return ""
	}
	return anim.Mode
}

// styleAttrs delegates to style.Style.attrs() to avoid duplicating SGR logic.
func styleAttrs(s style.Style) string { return s.Attrs() }

func blockValue(st status.StatusState, block layout.Block, templates map[string]TemplateSpec) string {
	if block.IsLiteral() {
		return block.Text // trusted user config, no sanitization
	}
	id := canonicalModuleID(block.ModuleID)
	if tmpl, ok := templates[id]; ok {
		return resolveTemplate(st, tmpl)
	}
	snapshot, ok := st.Modules[status.ModuleID(id)]
	if !ok || snapshot.Err != nil {
		return ""
	}
	return snapshotText(snapshot)
}

// snapshotText converts a snapshot value to its display string. Shared by
// blockValue and resolveTemplate so all value kinds render consistently.
func snapshotText(snap status.ModuleSnapshot) string {
	switch snap.Value.Kind {
	case status.KindText:
		return sanitizeDisplayText(snap.Value.Text)
	case status.KindNumber:
		return fmt.Sprint(snap.Value.Number)
	case status.KindBool:
		return fmt.Sprint(snap.Value.Bool)
	case status.KindStatus:
		if snap.Value.Status != nil {
			return sanitizeDisplayText(snap.Value.Status.Text)
		}
	}
	return ""
}

func sanitizeDisplayText(s string) string {
	for i, r := range s {
		if isTerminalControl(r) {
			var b strings.Builder
			b.Grow(len(s))
			b.WriteString(s[:i])
			for _, r2 := range s[i:] {
				if !isTerminalControl(r2) {
					b.WriteRune(r2)
				}
			}
			return b.String()
		}
	}
	return s
}

func isTerminalControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

func canonicalModuleID(id string) string {
	return id
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
