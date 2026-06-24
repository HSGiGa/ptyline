// Package style resolves a per-block visual style (colors, attributes, padding,
// separators, segment shape) from config into something the renderer can apply.
// Visual styles are terminal text, not a GUI: shapes are Unicode glyphs plus
// background colors and padding (spec §8.9).
package style

// Shape is a segment rendering style.
type Shape string

const (
	ShapeFlat      Shape = "flat"
	ShapePowerline Shape = "powerline"
	ShapePill      Shape = "pill"
	ShapeBox       Shape = "box"
)

// Style is the resolved appearance of one block.
type Style struct {
	FG, BG         string
	Bold           bool
	Dim            bool
	Italic         bool
	Underline      bool
	Shape          Shape
	LeftSeparator  string
	RightSeparator string
	PaddingLeft    int
	PaddingRight   int
}

// Apply wraps content in this style's escape sequences and padding.
//
// TODO scaffold (plan 10): emit fg/bg/attribute SGR sequences (resolved via the
// theme), padding, and separators for the chosen Shape, then reset.
func (s Style) Apply(content string) string {
	return content
}
