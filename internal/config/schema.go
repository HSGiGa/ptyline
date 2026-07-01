// Package config defines the TOML configuration schema, defaults, loading, and
// version migration. The bar layout is a structured block schema plus a small
// placeholder template — deliberately not Markdown (spec §13, ARCHITECTURE.md §17).
package config

import "fmt"

// CurrentVersion is the schema version written by this build. The loader
// migrates older configs forward before parsing (ARCHITECTURE.md §17).
const CurrentVersion = 1

// Config is the root configuration object.
type Config struct {
	Version int `toml:"config_version"`

	Shell             string `toml:"shell"` // "auto" | fish | bash | zsh | ...
	RefreshIntervalMS int    `toml:"refresh_interval_ms"`

	Bar     BarConfig               `toml:"bar"`
	Theme   ThemeConfig             `toml:"theme"`
	Icons   IconsConfig             `toml:"icons"`
	Modules map[string]ModuleConfig `toml:"module"`
	Styles  map[string]StyleConfig  `toml:"style"`
}

// BarConfig controls the reserved area and overall bar behavior.
type BarConfig struct {
	Format        string      `toml:"format"`
	Justify       string      `toml:"justify"`         // left | center | right | absolute_center
	MinBlockWidth int         `toml:"min_block_width"` // hide blocks narrower than this; 0 = disabled
	Rows          []RowConfig `toml:"row"`             // multi-line: one row per [[bar.row]] (takes precedence over Format)
	Separator     string      `toml:"separator"`
	Padding       int         `toml:"padding"`
}

// RowConfig is one row of a multi-line bar. Format uses the same placeholder
// template as Bar.Format: `{name}` blocks, `||` splitting into left/center/right
// anchors, and `|` marking a separator rendered as Separator. Fill is the
// character drawn in the gaps and caps around the blocks (default a space); set
// it to "-" for a dashes "border" row like `--{left} --- {center} --- {right} --`.
type RowConfig struct {
	Format    string `toml:"format"`
	Fill      string `toml:"fill"`
	Separator string `toml:"separator"`
}

// ModuleConfig is the per-module configuration (spec §8.7).
type ModuleConfig struct {
	Enabled             bool             `toml:"enabled"`
	Source              string           `toml:"source"` // time | exec | template; empty = builtin or exec for unknown IDs
	Format              string           `toml:"format"`
	Separator           string           `toml:"separator"`
	CollapseWhitespace  bool             `toml:"collapse_whitespace"`
	HideWhenEmpty       bool             `toml:"hide_when_empty"`
	Mode                string           `toml:"mode"`     // e.g. "shell-integration"
	Provider            string           `toml:"provider"` // command | osc | socket (future)
	Command             string           `toml:"command"`
	RefreshOnCommand    []string         `toml:"refresh_on_command"`
	RefreshOnCWD        bool             `toml:"refresh_on_cwd"` // exec: re-run when the shell's directory changes
	Env                 []string         `toml:"env"`
	IntervalMS          int              `toml:"interval_ms"`
	TimeoutMS           int              `toml:"timeout_ms"`
	MaxWidth            int              `toml:"max_width"`
	ActiveMinDurationMS int              `toml:"active_min_duration_ms"`
	DoneMinDurationMS   int              `toml:"done_min_duration_ms"`
	DoneSuccessTTLMS    int              `toml:"done_success_ttl_ms"`
	DoneFailureTTLMS    int              `toml:"done_failure_ttl_ms"`
	Animation           AnimationSetting `toml:"animation"` // false/none | true | glint | pulse | blink
	AnimationIntervalMS int              `toml:"animation_interval_ms"`
	Icon                string           `toml:"icon"`          // left | right; empty = no icon
	IconGlyph           string           `toml:"icon_glyph"`    // preferred Nerd Font glyph
	IconFallback        string           `toml:"icon_fallback"` // fallback text/glyph for non-Nerd presets
}

// AnimationSetting accepts the user-facing compact form:
//
//	animation = true        -> use the module's default animation behavior
//	animation = false/none  -> disable animation
//	animation = "pulse"     -> enable animation with an explicit effect
type AnimationSetting string

const (
	AnimationDefault AnimationSetting = "default"
	AnimationNone    AnimationSetting = "none"
)

func (a *AnimationSetting) UnmarshalTOML(v any) error {
	switch value := v.(type) {
	case bool:
		if value {
			*a = AnimationDefault
		} else {
			*a = AnimationNone
		}
		return nil
	case string:
		*a = AnimationSetting(value)
		return nil
	default:
		return fmt.Errorf("animation must be a boolean or string")
	}
}

func (a AnimationSetting) Enabled() bool {
	return a != "" && a != AnimationNone
}

func (a AnimationSetting) Effect() string {
	if a == AnimationDefault {
		return ""
	}
	return string(a)
}

// ThemeConfig selects color scheme, style preset, and semantic tokens (ARCHITECTURE.md §16).
type ThemeConfig struct {
	ColorScheme string            `toml:"color_scheme"`
	Style       string            `toml:"style"`
	Icons       string            `toml:"icons"`
	Fallback    string            `toml:"fallback"`
	Palette     map[string]string `toml:"palette"`
	Status      map[string]string `toml:"status"`
}

// IconsConfig controls icon preset and width handling (spec §8.10).
type IconsConfig struct {
	Enabled    bool   `toml:"enabled"`
	Preset     string `toml:"preset"`      // nerd-font | emoji | ascii
	EmojiWidth string `toml:"emoji_width"` // auto | 1 | 2
	Fallback   bool   `toml:"fallback"`
}

// StyleConfig is a per-block visual style (spec §8.9).
type StyleConfig struct {
	FG           string `toml:"fg"`
	BG           string `toml:"bg"`
	Bold         bool   `toml:"bold"`
	Dim          bool   `toml:"dim"`
	Italic       bool   `toml:"italic"`
	Underline    bool   `toml:"underline"`
	Animation    string `toml:"animation"` // glint | pulse | blink
	Shape        string `toml:"shape"`
	LeftCap      string `toml:"left_cap"`
	RightCap     string `toml:"right_cap"`
	PaddingLeft  int    `toml:"padding_left"`
	PaddingRight int    `toml:"padding_right"`
}
