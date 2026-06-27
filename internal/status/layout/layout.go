// Package layout is the small layout engine behind the bar. Even though the MVP
// renders a one-line format string, that string is parsed into renderable blocks
// with layout metadata so multi-line, priority overflow, and compact variants
// work later without a redesign (spec §8.8, arch.md §7, §8).
package layout

import (
	"sort"
	"strconv"
	"strings"
)

// Anchor is the terminal side a block is pinned to.
type Anchor string

const (
	AnchorLeft   Anchor = "left"
	AnchorCenter Anchor = "center"
	AnchorRight  Anchor = "right"
)

// Align is text alignment within a block's allocated area.
type Align string

const (
	AlignLeft   Align = "left"
	AlignCenter Align = "center"
	AlignRight  Align = "right"
)

// WidthKind enumerates width unit types (spec §8.8).
type WidthKind int

const (
	WidthAuto    WidthKind = iota // size to content
	WidthFill                     // take remaining space
	WidthCells                    // fixed N cells
	WidthPercent                  // N% of bar width
)

// Width is a resolved width spec.
type Width struct {
	Kind  WidthKind
	Value int // cells or percent depending on Kind
}

// Block is one renderable unit with layout metadata. Priority drives graceful
// degradation when the terminal is narrow (arch.md §8). A block is either a
// module reference (ModuleID set) or a literal run of text (Text set, ModuleID
// empty) produced by parsing the format string.
type Block struct {
	ModuleID string
	Text     string // literal content when ModuleID == ""
	Anchor   Anchor
	Align    Align
	Width    Width
	MinWidth int
	MaxWidth int
	Truncate string
	Priority int
	StyleID  string
}

// IsLiteral reports whether the block is a literal text run (no module).
func (b Block) IsLiteral() bool { return b.ModuleID == "" }

// Engine assigns each block a cell range given the total bar width.
type Engine struct {
	barWidth      int
	minBlockWidth int // 0 = disabled; hide blocks narrower than this threshold
}

// New creates a layout engine for a bar of the given cell width.
func New(barWidth int) *Engine { return &Engine{barWidth: barWidth} }

// NewWithMinBlock creates a layout engine that hides any block allocated fewer
// than minBlockWidth cells. Use this to prevent tiny truncated blocks when the
// terminal is narrow.
func NewWithMinBlock(barWidth, minBlockWidth int) *Engine {
	return &Engine{barWidth: barWidth, minBlockWidth: minBlockWidth}
}

// SetBarWidth updates the bar width (after a resize).
func (e *Engine) SetBarWidth(w int) { e.barWidth = w }

// BarWidth returns the configured bar width in cells.
func (e *Engine) BarWidth() int { return e.barWidth }

// Placement is the resolved position of a block on the bar row.
type Placement struct {
	Block    Block
	StartCol int // 0-based
	EndCol   int // exclusive
	Width    int // allocated cell width
	Visible  bool
}

// Arrange resolves widths and visibility for the blocks given their natural
// (content) display widths. Blocks are kept in priority order until the bar is
// full; the lowest-priority blocks are dropped so the visible total never exceeds
// barWidth (which would wrap the bar and corrupt the screen). StartCol/EndCol are
// filled in by the renderer once section strings are assembled.
func (e *Engine) Arrange(blocks []Block, natural []int) []Placement {
	return e.arrange(blocks, natural)
}

// ArrangeIn is like Arrange but uses effectiveWidth instead of the engine's
// barWidth. Use this when part of the bar is reserved for caps or decorations
// (e.g. border rows) so block widths are computed against the inner area only.
// The caller must not invoke ArrangeIn concurrently on the same engine.
func (e *Engine) ArrangeIn(blocks []Block, natural []int, effectiveWidth int) []Placement {
	saved := e.barWidth
	e.barWidth = effectiveWidth
	p := e.arrange(blocks, natural)
	e.barWidth = saved
	return p
}

func (e *Engine) arrange(blocks []Block, natural []int) []Placement {
	placements := make([]Placement, len(blocks))
	for i, b := range blocks {
		placements[i] = Placement{Block: b, Width: e.resolveWidth(b, natural[i])}
	}

	// Visit blocks highest-priority first (stable on document order) and keep each
	// while it still fits in the remaining width.
	order := make([]int, len(blocks))
	for i := range order {
		order[i] = i
	}
	// Sort highest-priority first. Within the same priority, left-anchored blocks
	// are kept before center, and center before right — so right-anchored blocks
	// are the first to be dropped when the bar is narrow.
	sort.SliceStable(order, func(a, b int) bool {
		pa, pb := blocks[order[a]].Priority, blocks[order[b]].Priority
		if pa != pb {
			return pa > pb
		}
		return anchorDropOrder(blocks[order[a]].Anchor) > anchorDropOrder(blocks[order[b]].Anchor)
	})

	remaining := e.barWidth
	for _, idx := range order {
		w := placements[idx].Width
		isFill := blocks[idx].Width.Kind == WidthFill
		tooNarrow := e.minBlockWidth > 0 && !isFill && w < e.minBlockWidth
		if w <= remaining && !tooNarrow {
			placements[idx].Visible = true
			remaining -= w
		}
	}

	e.distributeFill(placements)
	return placements
}

// distributeFill expands visible WidthFill blocks to share any leftover bar width
// equally (spec §8.8). Each fill block starts at its natural width; the remainder
// is split between them, honoring MaxWidth. Non-fill layouts leave this a no-op.
func (e *Engine) distributeFill(placements []Placement) {
	used, fills := 0, 0
	for i := range placements {
		if !placements[i].Visible {
			continue
		}
		used += placements[i].Width
		if placements[i].Block.Width.Kind == WidthFill {
			fills++
		}
	}
	if fills == 0 || used >= e.barWidth {
		return
	}
	extra := e.barWidth - used
	per, rem := extra/fills, extra%fills
	for i := range placements {
		if !placements[i].Visible || placements[i].Block.Width.Kind != WidthFill {
			continue
		}
		add := per
		if rem > 0 {
			add++
			rem--
		}
		w := placements[i].Width + add
		if mw := placements[i].Block.MaxWidth; mw > 0 && w > mw {
			w = mw
		}
		placements[i].Width = w
	}
}

// resolveWidth turns a block's width spec into a concrete cell count, clamped to
// the block's min/max.
func (e *Engine) resolveWidth(b Block, natural int) int {
	w := natural
	switch b.Width.Kind {
	case WidthCells:
		w = b.Width.Value
	case WidthPercent:
		w = e.barWidth * b.Width.Value / 100
	case WidthFill, WidthAuto:
		w = natural
	}
	if b.MinWidth > 0 && w < b.MinWidth {
		w = b.MinWidth
	}
	if b.MaxWidth > 0 && w > b.MaxWidth {
		w = b.MaxWidth
	}
	if w < 0 {
		w = 0
	}
	return w
}

// ParseFormat turns a placeholder template into ordered blocks. `||` splits the
// template into anchor sections (1 → left; 2 → left,right; 3 → left,center,right)
// and `{name}` placeholders become module blocks; the literal text between them
// becomes literal blocks (spec §13.1).
func ParseFormat(format string) []Block {
	sections := strings.Split(format, "||")
	anchors := sectionAnchors(len(sections))

	var blocks []Block
	for i, section := range sections {
		blocks = append(blocks, parseSection(section, anchors[i])...)
	}
	return blocks
}

func sectionAnchors(n int) []Anchor {
	switch n {
	case 1:
		return []Anchor{AnchorLeft}
	case 2:
		return []Anchor{AnchorLeft, AnchorRight}
	default: // 3 or more: extra sections fold into the right anchor
		out := []Anchor{AnchorLeft, AnchorCenter, AnchorRight}
		for len(out) < n {
			out = append(out, AnchorRight)
		}
		return out
	}
}

// parseSection splits a single section into literal and {module} blocks.
func parseSection(section string, anchor Anchor) []Block {
	var blocks []Block
	i := 0
	for i < len(section) {
		open := strings.IndexByte(section[i:], '{')
		if open < 0 {
			blocks = append(blocks, literalBlock(section[i:], anchor))
			break
		}
		open += i
		if open > i {
			blocks = append(blocks, literalBlock(section[i:open], anchor))
		}
		close := strings.IndexByte(section[open:], '}')
		if close < 0 {
			// Unterminated placeholder: treat the rest as literal.
			blocks = append(blocks, literalBlock(section[open:], anchor))
			break
		}
		close += open
		blocks = append(blocks, placeholderBlock(section[open+1:close], anchor))
		i = close + 1
	}
	return blocks
}

func placeholderBlock(expr string, anchor Anchor) Block {
	name, spec, hasSpec := strings.Cut(expr, ":")
	block := Block{
		ModuleID: name,
		Anchor:   anchor,
		Align:    AlignLeft,
		Width:    Width{Kind: WidthAuto},
		Truncate: "right",
	}
	if !hasSpec || len(spec) < 2 {
		return block
	}
	cells, err := strconv.Atoi(spec[1:])
	if err != nil || cells <= 0 {
		return block
	}
	switch spec[0] {
	case '<':
		block.Align = AlignLeft
	case '^':
		block.Align = AlignCenter
	case '>':
		block.Align = AlignRight
	default:
		return block
	}
	block.Width = Width{Kind: WidthCells, Value: cells}
	return block
}

func literalBlock(text string, anchor Anchor) Block {
	return Block{
		Text:     text,
		Anchor:   anchor,
		Align:    AlignLeft,
		Width:    Width{Kind: WidthAuto},
		Truncate: "none",
		// Literals (separators, spacing) are kept ahead of modules under pressure.
		Priority: 1,
	}
}

// anchorDropOrder returns a tiebreaker value used in arrange's sort: higher =
// kept earlier when width runs out. Right-anchored blocks are dropped first
// (value 0), then center (1), then left (2).
func anchorDropOrder(a Anchor) int {
	switch a {
	case AnchorLeft:
		return 2
	case AnchorCenter:
		return 1
	default: // AnchorRight
		return 0
	}
}
