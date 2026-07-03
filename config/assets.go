// Package assets embeds the color schemes, style presets, and sample config
// shipped with the binary. Themes and styles are resolved from the user's
// config directory first and fall back to these built-ins, so a fresh install
// renders correctly without copying any files into ~/.config (see
// internal/app/bar.loadThemeFile). The sample config is written to the user's
// config path on first run (see config.EnsureUserConfig).
package assets

import "embed"

// Themes holds the built-in color schemes, keyed as themes/<name>.toml.
//
//go:embed themes/*.toml
var Themes embed.FS

// Styles holds the built-in style presets, keyed as styles/<name>.toml.
//
//go:embed styles/*.toml
var Styles embed.FS

//go:embed config.toml
var sampleConfig []byte

// SampleConfig returns the sample config.toml shipped with this build.
func SampleConfig() []byte { return sampleConfig }
