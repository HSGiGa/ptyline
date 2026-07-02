// Package layout is the small layout engine behind the bar. Even though the MVP
// renders a one-line format string, that string is parsed into renderable blocks
// with layout metadata so multi-line, priority overflow, and compact variants
// work later without a redesign (spec §8.8, ARCHITECTURE.md §7, §8).
//
// Block, Anchor, Align, Width, WidthKind, and ParseFormat live in the sibling
// package internal/format so that config validation can import them without
// creating a config → status/layout dependency cycle.
package layout

import (
	"sort"

	"github.com/hsgiga/ptyline/internal/format"
)

// Type aliases let all existing callers continue to use layout.Block,
// layout.Anchor, etc. without change, while the canonical definitions live in
// the leaf package internal/format.
type (
	Block     = format.Block
	Anchor    = format.Anchor
	Align     = format.Align
	Width     = format.Width
	WidthKind = format.WidthKind
)

// Re-export Anchor constants so callers need not import internal/format directly.
const (
	AnchorLeft   Anchor = format.AnchorLeft
	AnchorCenter Anchor = format.AnchorCenter
	AnchorRight  Anchor = format.AnchorRight
)

// Re-export Align constants.
const (
	AlignLeft   Align = format.AlignLeft
	AlignCenter Align = format.AlignCenter
	AlignRight  Align = format.AlignRight
)

// Re-export WidthKind constants.
const (
	WidthAuto    = format.WidthAuto
	WidthFill    = format.WidthFill
	WidthCells   = format.WidthCells
	WidthPercent = format.WidthPercent
)

// ParseFormat delegates to the format package. It is kept here for backward
// compatibility; prefer calling format.ParseFormat directly in new code.
func ParseFormat(s string) []Block { return format.ParseFormat(s) }

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
		tooNarrow := e.minBlockWidth > 0 && !isFill && !blocks[idx].IsLiteral() && !blocks[idx].IsSeparator() && w < e.minBlockWidth
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
