package renderer

import (
	"math"
	"strings"

	"github.com/hsgiga/ptyline/internal/status/style"
	"github.com/hsgiga/ptyline/internal/status/theme"
)

// Hardcoded animation catalog. These are the only valid values for
// ModuleConfig.Animation. All effects are color-only: display width is never
// changed, so layout calculations remain correct.
const (
	AnimGlint = "glint" // warm shimmer sweeps across text per-cell
	AnimPulse = "pulse" // whole block breathes dim↔bright via sine wave
	AnimBlink = "blink" // whole block alternates normal↔dim at slow cadence
)

// glintHighlight is the warm color the shimmer blends the base FG toward at its
// brightest. glintHalfWidth is how many cells the glow fades out over on each
// side of the bright center — a wide, soft falloff reads as a smooth glide.
var glintHighlight = theme.RGB{R: 0xff, G: 0xf0, B: 0xc2}

const glintHalfWidth = 3

// applyGlint renders the block as a seamless shimmer: a soft brightness wave
// glides across the text and wraps on a ring of the text length, so the glow
// leaving the right edge re-enters from the left with no gap and no snap. The
// per-cell color is the base FG blended toward glintHighlight by a distance
// falloff; display width is untouched (only colors change).
func (r *Renderer) applyGlint(content string, s style.Style, phase int) string {
	body := strings.Repeat(" ", max(0, s.PaddingLeft)) + content + strings.Repeat(" ", max(0, s.PaddingRight))
	if r.theme == nil || r.theme.Mode() == theme.NoColor {
		return s.LeftCap + body + s.RightCap
	}
	runes := []rune(body)
	if len(runes) == 0 {
		return s.Apply(content, r.theme)
	}
	base, ok := r.theme.Resolve(s.FG)
	var b strings.Builder
	b.WriteString(s.LeftCap)
	b.WriteString(r.theme.BG(s.BG))
	b.WriteString(styleAttrs(s))
	if !ok {
		// Unknown base color: keep the static FG so the text still renders.
		fg := r.theme.FG(s.FG)
		for _, ch := range runes {
			b.WriteString(fg)
			b.WriteRune(ch)
		}
	} else {
		l := len(runes)
		if phase < 0 {
			phase = -phase
		}
		center := phase % l
		// Pre-compute the color palette (glintHalfWidth distinct blended colors)
		// so the hot per-glyph loop only indexes, avoiding repeated Sprintf calls.
		var palette [glintHalfWidth]string
		for d := 0; d < glintHalfWidth; d++ {
			t := 1 - float64(d)/glintHalfWidth
			palette[d] = r.theme.FGRGB(mixRGB(base, glintHighlight, t))
		}
		baseSGR := r.theme.FGRGB(base)
		for i, ch := range runes {
			d := circularDistance(i, center, l)
			if d >= glintHalfWidth {
				b.WriteString(baseSGR)
			} else {
				b.WriteString(palette[d])
			}
			b.WriteRune(ch)
		}
	}
	b.WriteString(theme.Reset)
	b.WriteString(s.RightCap)
	return b.String()
}

// applyPulse renders the block with a smooth sinusoidal brightness cycle: the
// FG color blends from base toward a dimmed shade and back. The whole block
// changes uniformly — no per-cell work. Display width is untouched.
func (r *Renderer) applyPulse(content string, s style.Style, phase int) string {
	if r.theme == nil || r.theme.Mode() == theme.NoColor {
		return s.Apply(content, r.theme)
	}
	base, ok := r.theme.Resolve(s.FG)
	if !ok {
		return s.Apply(content, r.theme)
	}
	// sin oscillates between -1 and 1; map to t ∈ [0.25, 1.0] so the text
	// never fully disappears — just breathes between full and quarter-bright.
	const period = 16.0
	t := 0.625 + 0.375*math.Sin(float64(phase)*2*math.Pi/period)
	dim := theme.RGB{
		R: uint8(float64(base.R) * 0.35),
		G: uint8(float64(base.G) * 0.35),
		B: uint8(float64(base.B) * 0.35),
	}
	blended := mixRGB(dim, base, t)
	body := strings.Repeat(" ", max(0, s.PaddingLeft)) + content + strings.Repeat(" ", max(0, s.PaddingRight))
	var b strings.Builder
	b.WriteString(s.LeftCap)
	b.WriteString(r.theme.BG(s.BG))
	b.WriteString(styleAttrs(s))
	b.WriteString(r.theme.FGRGB(blended))
	b.WriteString(body)
	b.WriteString(theme.Reset)
	b.WriteString(s.RightCap)
	return b.String()
}

// applyBlink alternates the block between normal and dim on a slow cadence.
// Uses SGR Dim (2) rather than hiding the text, so the block stays readable.
// Display width is untouched.
const blinkPeriod = 8 // ticks per half-cycle

func (r *Renderer) applyBlink(content string, s style.Style, phase int) string {
	dimPhase := s
	if (phase/blinkPeriod)%2 != 0 {
		dimPhase.Dim = true
	}
	return dimPhase.Apply(content, r.theme)
}

// circularDistance is the shorter distance between i and center on a ring of
// length l, so the shimmer wraps seamlessly across the text edges.
func circularDistance(i, center, l int) int {
	forward := ((i-center)%l + l) % l
	backward := ((center-i)%l + l) % l
	if forward < backward {
		return forward
	}
	return backward
}

// mixRGB linearly interpolates each channel from a toward b by t in [0,1].
func mixRGB(a, b theme.RGB, t float64) theme.RGB {
	lerp := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5)
	}
	return theme.RGB{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B)}
}
