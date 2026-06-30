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
	AnimGlint = "glint" // soft shimmer sweeps across text per-cell
	AnimPulse = "pulse" // whole block breathes dim↔bright via sine wave
	AnimBlink = "blink" // whole block alternates normal↔dim at slow cadence
)

// glintHalfWidth is how many cells the glow fades out over on each side of the
// bright center. A wider falloff reads as a softer shimmer rather than a hard
// single-cell scanline.
const glintHalfWidth = 5

// applyGlint renders the block as a single soft shimmer pass: the glow enters
// from the left, traverses the text, fully exits on the right, and only then
// starts the next pass. The per-cell color blends from a dimmed foreground
// toward the configured foreground by a smooth distance falloff; display width
// is untouched.
func (r *Renderer) applyGlint(content string, s style.Style, phase int) string {
	body := strings.Repeat(" ", max(0, s.PaddingLeft)) + content + strings.Repeat(" ", max(0, s.PaddingRight))
	if r.theme == nil || r.theme.Mode() == theme.NoColor {
		return s.LeftCap + body + s.RightCap
	}
	runes := []rune(body)
	if len(runes) == 0 {
		return s.Apply(content, r.theme)
	}
	highlight, ok := r.glintForegroundColor(s)
	if !ok {
		return s.Apply(content, r.theme)
	}
	base := glintDimColor(highlight)
	var b strings.Builder
	b.WriteString(s.LeftCap)
	b.WriteString(r.theme.BG(s.BG))
	b.WriteString(styleAttrs(s))
	center := glintCenter(phase, len(runes))
	// Pre-compute the color palette (glintHalfWidth distinct blended colors) so
	// the hot per-glyph loop only indexes, avoiding repeated Sprintf calls.
	var palette [glintHalfWidth]string
	for d := 0; d < glintHalfWidth; d++ {
		t := 1 - float64(d)/float64(glintHalfWidth)
		t = smoothstep(t)
		palette[d] = r.theme.FGRGB(mixRGB(base, highlight, t))
	}
	baseSGR := r.theme.FGRGB(base)
	for i, ch := range runes {
		d := absInt(i - center)
		if d >= glintHalfWidth {
			b.WriteString(baseSGR)
		} else {
			b.WriteString(palette[d])
		}
		b.WriteRune(ch)
	}
	b.WriteString(theme.Reset)
	b.WriteString(s.RightCap)
	return b.String()
}

func glintCenter(phase, length int) int {
	if phase < 0 {
		phase = -phase
	}
	cycle := glintCycleLength(length)
	if cycle <= 0 {
		return -glintHalfWidth
	}
	return phase%cycle - glintHalfWidth
}

func glintCycleLength(length int) int {
	return length + 2*glintHalfWidth
}

func glintVisible(content string, s style.Style, phase int) bool {
	body := strings.Repeat(" ", max(0, s.PaddingLeft)) + content + strings.Repeat(" ", max(0, s.PaddingRight))
	length := len([]rune(body))
	if length == 0 {
		return false
	}
	center := glintCenter(phase, length)
	return center+glintHalfWidth > 0 && center-glintHalfWidth < length
}

func (r *Renderer) applyGlintDim(content string, s style.Style) string {
	body := strings.Repeat(" ", max(0, s.PaddingLeft)) + content + strings.Repeat(" ", max(0, s.PaddingRight))
	if r.theme == nil || r.theme.Mode() == theme.NoColor {
		return s.LeftCap + body + s.RightCap
	}
	highlight, ok := r.glintForegroundColor(s)
	if !ok {
		return s.LeftCap + body + s.RightCap
	}
	fg := glintDimColor(highlight)
	var b strings.Builder
	b.WriteString(s.LeftCap)
	b.WriteString(r.theme.BG(s.BG))
	b.WriteString(styleAttrs(s))
	b.WriteString(r.theme.FGRGB(fg))
	b.WriteString(body)
	b.WriteString(theme.Reset)
	b.WriteString(s.RightCap)
	return b.String()
}

func (r *Renderer) glintForegroundColor(s style.Style) (theme.RGB, bool) {
	if s.FG != "" {
		if base, ok := r.theme.Resolve(s.FG); ok {
			return base, true
		}
	}
	if base, ok := r.theme.Resolve("base.fg"); ok {
		return base, true
	}
	// The terminal-native default theme exposes no base.fg (the terminal's own
	// default foreground is used as-is), so a module with no explicit FG — notably
	// {command} — would leave glint with no concrete RGB to shimmer toward and the
	// effect would silently degrade to plain text. Fall back to a neutral gray-white
	// so the shimmer stays visible and theme-agnostic: glintDimColor takes it down to
	// a mid gray, giving a grayscale white↔gray sweep.
	return glintFallbackColor, true
}

// glintFallbackColor is the highlight glint shimmers toward when neither the
// module's FG nor base.fg resolves to a concrete color (e.g. the terminal-native
// default theme). A light gray-white reads as a neutral shimmer on any background.
var glintFallbackColor = theme.RGB{R: 0xcc, G: 0xcc, B: 0xcc}

func glintDimColor(base theme.RGB) theme.RGB {
	return theme.RGB{
		R: uint8(float64(base.R) * 0.42),
		G: uint8(float64(base.G) * 0.42),
		B: uint8(float64(base.B) * 0.42),
	}
}

func smoothstep(t float64) float64 {
	return t * t * (3 - 2*t)
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

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// mixRGB linearly interpolates each channel from a toward b by t in [0,1].
func mixRGB(a, b theme.RGB, t float64) theme.RGB {
	lerp := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5)
	}
	return theme.RGB{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B)}
}
