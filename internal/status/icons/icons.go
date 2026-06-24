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

// Set resolves icons for the active preset, falling back when configured.
type Set struct {
	Preset   Preset
	Fallback bool
}

// New creates an icon set.
func New(preset Preset, fallback bool) Set {
	return Set{Preset: preset, Fallback: fallback}
}

// Icon returns the glyph for a module under the active preset. `primary` is the
// preferred glyph and `fallback` the ASCII-safe alternative (spec §8.10).
//
// TODO scaffold (plan 10): choose based on Preset and capability-detected font
// support; respect the emoji-width policy for measurement.
func (s Set) Icon(primary, fallback string) string {
	if s.Preset == PresetASCII || (s.Fallback && primary == "") {
		return fallback
	}
	return primary
}
