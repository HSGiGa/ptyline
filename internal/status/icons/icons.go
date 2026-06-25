// Package icons resolves module icons across presets. Icons and emoji are just
// text rendered by the terminal's own font; ptyline never bundles or switches
// fonts. If a requested glyph is unavailable, a fallback icon keeps the bar
// usable (spec §8.10).
package icons

// Preset selects the glyph family.
type Preset string

const (
	PresetNerdFont Preset = "nerd-font"
	PresetEmoji    Preset = "emoji"
	PresetASCII    Preset = "ascii"
)

// EmojiWidth is the cell-width policy for emoji glyphs (spec §8.10). Some
// terminals render emoji single-width, others double; "auto" defers to the width
// measurement, while "1"/"2" force a value.
type EmojiWidth string

const (
	EmojiWidthAuto   EmojiWidth = "auto"
	EmojiWidthSingle EmojiWidth = "1"
	EmojiWidthDouble EmojiWidth = "2"
)

// Set resolves icons for the active preset, falling back when configured.
type Set struct {
	Preset     Preset
	Fallback   bool
	EmojiWidth EmojiWidth
}

// New creates an icon set.
func New(preset Preset, fallback bool) Set {
	return Set{Preset: preset, Fallback: fallback, EmojiWidth: EmojiWidthAuto}
}

// Icon returns the glyph for a module under the active preset. `primary` is the
// preferred glyph and `fallback` the ASCII-safe alternative (spec §8.10). The
// ASCII preset always uses the fallback, and an empty primary is unusable so it
// also falls back — the bar stays readable without Nerd Fonts or emoji.
func (s Set) Icon(primary, fallback string) string {
	if s.Preset == PresetASCII || primary == "" {
		return fallback
	}
	return primary
}

// ForcedWidth returns the cell width to assume for an emoji glyph, or 0 when the
// policy is "auto" (measure normally). Only meaningful for the emoji preset.
func (s Set) ForcedWidth() int {
	switch s.EmojiWidth {
	case EmojiWidthSingle:
		return 1
	case EmojiWidthDouble:
		return 2
	default:
		return 0
	}
}
