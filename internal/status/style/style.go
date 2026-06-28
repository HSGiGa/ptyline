// Package style resolves a per-block visual style (colors, attributes, padding,
// caps, segment shape) into the escape sequences the renderer applies.
// Visual styles are terminal text, not a GUI: shapes are Unicode glyphs plus
// background colors and padding (spec §8.9).
package style

import (
	"strings"

	"github.com/hsgiga/ptyline/internal/status/theme"
	"github.com/hsgiga/ptyline/internal/status/width"
)

// Shape is a segment rendering style.
type Shape string

const (
	ShapeFlat      Shape = "flat"
	ShapePowerline Shape = "powerline"
	ShapePill      Shape = "pill"
	ShapeBox       Shape = "box"
)

// Style is the resolved appearance of one block. FG/BG are color references the
// theme resolves (a token like "accent", a "#rrggbb" literal, or a named color).
type Style struct {
	FG, BG       string
	Bold         bool
	Dim          bool
	Italic       bool
	Underline    bool
	Animation    string
	Shape        Shape
	LeftCap      string
	RightCap     string
	PaddingLeft  int
	PaddingRight int
}

// Apply wraps content in this style's escape sequences and padding, resetting at
// the end so styling never leaks into the child output (spec §20.14). In
// no-color mode (nil theme or theme.NoColor) it emits plain text with padding and
// caps only. Only the flat shape is implemented for the MVP; powerline/
// pill/box are post-MVP (spec §19) and currently render as flat.
func (s Style) Apply(content string, th *theme.Theme) string {
	body := s.Padded(content)
	if th == nil || th.Mode() == theme.NoColor {
		return s.Plain(content)
	}
	var b strings.Builder
	b.WriteString(s.LeftCap)
	b.WriteString(th.FG(s.FG))
	b.WriteString(th.BG(s.BG))
	b.WriteString(s.Attrs())
	b.WriteString(body)
	b.WriteString(theme.Reset)
	b.WriteString(s.RightCap)
	return b.String()
}

// Plain returns the visible cells produced by this style without ANSI escapes.
func (s Style) Plain(content string) string {
	return s.LeftCap + s.Padded(content) + s.RightCap
}

// OuterWidth returns the visible width added around content by caps and
// padding. It intentionally excludes the content itself.
func (s Style) OuterWidth() int {
	return width.String(s.LeftCap) + max(0, s.PaddingLeft) + max(0, s.PaddingRight) + width.String(s.RightCap)
}

// Padded returns content with this style's inner spacing applied.
func (s Style) Padded(content string) string {
	var b strings.Builder
	if s.PaddingLeft > 0 {
		b.WriteString(strings.Repeat(" ", s.PaddingLeft))
	}
	b.WriteString(content)
	if s.PaddingRight > 0 {
		b.WriteString(strings.Repeat(" ", s.PaddingRight))
	}
	return b.String()
}

// Attrs renders the SGR sequence for the enabled text attributes, or "" if none.
func (s Style) Attrs() string {
	var b strings.Builder
	first := true
	emit := func(code string) {
		if first {
			b.WriteString("\x1b[")
			first = false
		} else {
			b.WriteByte(';')
		}
		b.WriteString(code)
	}
	if s.Bold {
		emit("1")
	}
	if s.Dim {
		emit("2")
	}
	if s.Italic {
		emit("3")
	}
	if s.Underline {
		emit("4")
	}
	if first {
		return ""
	}
	b.WriteByte('m')
	return b.String()
}
