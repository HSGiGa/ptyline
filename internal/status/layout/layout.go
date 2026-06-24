// Package layout is the small layout engine behind the bar. Even though the MVP
// renders a one-line format string, that string is parsed into renderable blocks
// with layout metadata so multi-line, priority overflow, and compact variants
// work later without a redesign (spec §8.8, arch.md §7, §8).
package layout

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
// degradation when the terminal is narrow (arch.md §8).
type Block struct {
	ModuleID string
	Anchor   Anchor
	Align    Align
	Width    Width
	MinWidth int
	MaxWidth int
	Truncate string
	Priority int
	StyleID  string
}

// Engine assigns each block a cell range given the total bar width.
type Engine struct {
	barWidth int
}

// New creates a layout engine for a bar of the given cell width.
func New(barWidth int) *Engine { return &Engine{barWidth: barWidth} }

// Placement is the resolved position of a block on the bar row.
type Placement struct {
	Block    Block
	StartCol int // 0-based
	EndCol   int // exclusive
	Visible  bool
}

// Arrange resolves widths and positions for the blocks, dropping/compacting the
// lowest-priority blocks when space runs out.
//
// TODO scaffold (plan 09): implement three-section (left/center/right) packing,
// percent/fill resolution, min/max clamping, and priority-based overflow.
func (e *Engine) Arrange(blocks []Block) []Placement {
	_ = e.barWidth
	out := make([]Placement, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, Placement{Block: b})
	}
	return out
}
