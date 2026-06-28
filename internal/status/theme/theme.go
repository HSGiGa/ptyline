// Package theme translates semantic color tokens into terminal escape sequences.
// Modules and blocks request tokens (accent/ok/warn/error/muted, base.fg/base.bg)
// or literal colors rather than writing raw ANSI, so light/dark, no-color, and
// reduced-color terminals all work uniformly (ARCHITECTURE.md §16).
package theme

import (
	"fmt"
	"strconv"
	"strings"
)

// Reset clears all SGR attributes. The renderer/style layer emits it to end a
// styled run; the serialized writer also resets after each bar frame.
const Reset = "\x1b[0m"

// Mode is the color depth the theme renders for. It mirrors
// runtimeenv.ColorLevel; the app maps one to the other so this package stays
// free of a runtimeenv import.
type Mode int

const (
	NoColor   Mode = iota // emit no color escapes at all
	Color16               // nearest of the 16 ANSI colors
	Color256              // 256-color palette
	TrueColor             // 24-bit color
)

// RGB is a 24-bit color. Tokens resolve to RGB; rendering then degrades to the
// active Mode.
type RGB struct{ R, G, B uint8 }

// Theme resolves color references for one Mode.
type Theme struct {
	mode    Mode
	palette map[string]RGB
}

// New builds a theme for a mode over a token→color palette.
func New(mode Mode, palette map[string]RGB) *Theme {
	return &Theme{mode: mode, palette: palette}
}

// DefaultPalette returns the built-in terminal-native palette. It uses only the
// 16 ANSI system colors that every terminal theme remaps, with no explicit
// background, so the bar inherits the terminal emulator's own color scheme.
func DefaultPalette() map[string]RGB {
	return map[string]RGB{
		// No base.bg / base.fg: terminal default colors are used as-is.
		// Standard ANSI 0-7 colors match shell prompt conventions (bash/zsh/fish use
		// bold + standard color, not bright variants, for user@host / cwd / errors).
		"accent": namedColors["brightcyan"],  // 14 — accent highlights, SSH
		"ok":     namedColors["green"],       // 2  — git clean, hostname (matches bash \u@\h bold green)
		"warn":   namedColors["yellow"],      // 3  — git modified, time
		"error":  namedColors["red"],         // 1  — exit code, git conflict
		"muted":  namedColors["brightblack"], // 8  — separators, frame chrome (needs gray, not black)
	}
}

// Default returns the built-in terminal-native palette.
// It is legible without Nerd Fonts or emoji (spec §20.15).
func Default(mode Mode) *Theme {
	return New(mode, DefaultPalette())
}

// Mode reports the color depth this theme renders for.
func (t *Theme) Mode() Mode { return t.mode }

// FG returns the SGR sequence setting the foreground for a color reference, or
// "" in no-color mode / for an unknown reference.
func (t *Theme) FG(ref string) string { return t.sgr(ref, 38) }

// BG returns the SGR sequence setting the background for a color reference.
func (t *Theme) BG(ref string) string { return t.sgr(ref, 48) }

// Resolve maps a color reference (palette token, "#rrggbb" literal, or named ANSI
// color) to its RGB value. It lets callers interpolate between concrete colors —
// e.g. animation effects that blend a base color toward a highlight.
func (t *Theme) Resolve(ref string) (RGB, bool) { return t.resolve(ref) }

// ResolveInPalette maps a color reference using an explicit palette. It accepts
// palette tokens, "#rrggbb" literals, and named ANSI colors.
func ResolveInPalette(ref string, palette map[string]RGB) (RGB, bool) {
	if c, ok := palette[ref]; ok {
		return c, true
	}
	if strings.HasPrefix(ref, "#") {
		return parseHex(ref)
	}
	if c, ok := namedColors[strings.ToLower(ref)]; ok {
		return c, true
	}
	return RGB{}, false
}

// FGRGB returns the SGR sequence setting the foreground to an arbitrary RGB,
// degraded to the active Mode, or "" in no-color mode.
func (t *Theme) FGRGB(c RGB) string {
	if t.mode == NoColor {
		return ""
	}
	return c.sgr(38, t.mode)
}

func (t *Theme) sgr(ref string, layer int) string {
	if t.mode == NoColor || ref == "" {
		return ""
	}
	c, ok := t.resolve(ref)
	if !ok {
		return ""
	}
	return c.sgr(layer, t.mode)
}

// resolve maps a color reference to an RGB value. References are, in order: a
// palette token, a "#rrggbb" literal, or a named ANSI color.
func (t *Theme) resolve(ref string) (RGB, bool) {
	return ResolveInPalette(ref, t.palette)
}

func parseHex(s string) (RGB, bool) {
	if len(s) != 7 {
		return RGB{}, false
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return RGB{}, false
	}
	return RGB{uint8(v >> 16), uint8(v >> 8), uint8(v)}, true
}

// sgr renders the color for the given layer (38 fg / 48 bg) and mode.
func (c RGB) sgr(layer int, mode Mode) string {
	switch mode {
	case TrueColor:
		return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", layer, c.R, c.G, c.B)
	case Color256:
		return fmt.Sprintf("\x1b[%d;5;%dm", layer, c.index256())
	case Color16:
		return fmt.Sprintf("\x1b[%dm", c.code16(layer))
	default:
		return ""
	}
}

// index256 maps RGB onto the xterm 256-color cube (16–231) or grayscale ramp
// (232–255), whichever is closer.
func (c RGB) index256() int {
	q := func(v uint8) int {
		switch {
		case v < 48:
			return 0
		case v < 115:
			return 1
		default:
			return (int(v) - 35) / 40
		}
	}
	r, g, b := q(c.R), q(c.G), q(c.B)
	cube := 16 + 36*r + 6*g + b
	// Compare against the nearest grayscale step.
	gray := (int(c.R) + int(c.G) + int(c.B)) / 3
	grayIdx := 232 + (gray-8)/10
	if grayIdx < 232 {
		grayIdx = 232
	}
	if grayIdx > 255 {
		grayIdx = 255
	}
	levels := []int{0, 95, 135, 175, 215, 255}
	cubeVal := RGB{uint8(levels[r]), uint8(levels[g]), uint8(levels[b])}
	grayVal := uint8(8 + 10*(grayIdx-232))
	if dist(c, cubeVal) <= dist(c, RGB{grayVal, grayVal, grayVal}) {
		return cube
	}
	return grayIdx
}

// code16 maps RGB onto the nearest of the 16 ANSI colors, returning the SGR
// parameter for the given layer (foreground 30–37/90–97, background +10).
func (c RGB) code16(layer int) int {
	best, bestDist := 0, 1<<31-1
	for i, pc := range ansi16 {
		if d := dist(c, pc); d < bestDist {
			best, bestDist = i, d
		}
	}
	base := 30
	if best >= 8 {
		base = 90 - 8 // bright colors 90–97
	}
	if layer == 48 {
		base += 10
	}
	return base + best
}

func dist(a, b RGB) int {
	dr, dg, db := int(a.R)-int(b.R), int(a.G)-int(b.G), int(a.B)-int(b.B)
	return dr*dr + dg*dg + db*db
}

// ansi16 are the standard xterm RGB values for the 16 base colors, indexed by
// ANSI color number (0–7 normal, 8–15 bright).
var ansi16 = [16]RGB{
	{0, 0, 0},
	{205, 0, 0},
	{0, 205, 0},
	{205, 205, 0},
	{0, 0, 238},
	{205, 0, 205},
	{0, 205, 205},
	{229, 229, 229},
	{127, 127, 127},
	{255, 0, 0},
	{0, 255, 0},
	{255, 255, 0},
	{92, 92, 255},
	{255, 0, 255},
	{0, 255, 255},
	{255, 255, 255},
}

var namedColors = map[string]RGB{
	"black": ansi16[0], "red": ansi16[1], "green": ansi16[2], "yellow": ansi16[3],
	"blue": ansi16[4], "magenta": ansi16[5], "cyan": ansi16[6], "white": ansi16[7],
	"brightblack": ansi16[8], "brightred": ansi16[9], "brightgreen": ansi16[10],
	"brightyellow": ansi16[11], "brightblue": ansi16[12], "brightmagenta": ansi16[13],
	"brightcyan": ansi16[14], "brightwhite": ansi16[15],
}
