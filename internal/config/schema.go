// Package config defines the TOML configuration schema, defaults, loading, and
// version migration. The bar layout is a structured block schema plus a small
// placeholder template — deliberately not Markdown (spec §13, arch.md §17).
package config

// CurrentVersion is the schema version written by this build. The loader
// migrates older configs forward before parsing (arch.md §17).
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
//
// MVP contract (spec §13.1): `Height` must be 1 in the normal screen; `Format`
// and `Blocks` are mutually exclusive (if Blocks is present, Format is rejected).
// Alternate-screen behavior is fixed by spec §11 (bar hidden) and is NOT
// configurable in the MVP — `ShowInAlternateScreen` is reserved for post-MVP
// (spec §19 "optional visible bar in alternate screen").
type BarConfig struct {
	Height        uint16        `toml:"height"`
	MaxHeight     uint16        `toml:"max_height"` // reserved (multi-line, post-MVP)
	Mode          string        `toml:"mode"`       // single-line | agent-panel (future)
	Format        string        `toml:"format"`
	Justify       string        `toml:"justify"`         // left | center | right | absolute_center
	MinBlockWidth int           `toml:"min_block_width"` // hide blocks narrower than this; 0 = disabled
	Rows          []RowConfig   `toml:"row"`             // multi-line: one row per [[bar.row]] (takes precedence over Format)
	Separator     string        `toml:"separator"`
	Padding       int           `toml:"padding"`
	Blocks        []BlockConfig `toml:"block"`

	ShowInAlternateScreen bool `toml:"show_in_alternate_screen"` // reserved; post-MVP
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

// BlockConfig is one layout block (spec §8.8).
type BlockConfig struct {
	Module   string `toml:"module"`
	Anchor   string `toml:"anchor"` // left | center | right
	Align    string `toml:"align"`  // left | center | right
	Width    string `toml:"width"`  // auto | fill | N | N%
	MinWidth string `toml:"min_width"`
	MaxWidth string `toml:"max_width"`
	Truncate string `toml:"truncate"` // left | right | middle | none
	Priority int    `toml:"priority"`
	Style    string `toml:"style"`
}

// ModuleConfig is the per-module configuration (spec §8.7).
type ModuleConfig struct {
	Enabled             bool     `toml:"enabled"`
	Source              string   `toml:"source"` // time | exec | template; empty = builtin or exec for unknown IDs
	Format              string   `toml:"format"`
	CollapseWhitespace  bool     `toml:"collapse_whitespace"`
	HideWhenEmpty       bool     `toml:"hide_when_empty"`
	Mode                string   `toml:"mode"`     // e.g. "shell-integration"
	Provider            string   `toml:"provider"` // command | osc | socket (future)
	Command             string   `toml:"command"`
	RefreshOnCommand    []string `toml:"refresh_on_command"`
	Env                 []string `toml:"env"`
	IntervalMS          int      `toml:"interval_ms"`
	TimeoutMS           int      `toml:"timeout_ms"`
	MaxWidth            int      `toml:"max_width"`
	DoneMinDurationMS   int      `toml:"done_min_duration_ms"`
	DoneSuccessTTLMS    int      `toml:"done_success_ttl_ms"`
	DoneFailureTTLMS    int      `toml:"done_failure_ttl_ms"`
	Animation           string   `toml:"animation"` // none | glint | pulse | blink
	AnimationIntervalMS int      `toml:"animation_interval_ms"`
	Icon                string   `toml:"icon"`          // left | right; empty = no icon
	IconGlyph           string   `toml:"icon_glyph"`    // preferred Nerd Font glyph
	IconFallback        string   `toml:"icon_fallback"` // fallback text/glyph for non-Nerd presets
}

// ThemeConfig selects color scheme, style preset, and semantic tokens (arch.md §16).
type ThemeConfig struct {
	ColorScheme string            `toml:"color_scheme"`
	Style       string            `toml:"style"`
	Icons       string            `toml:"icons"`
	Fallback    string            `toml:"fallback"`
	Palette     map[string]string `toml:"palette"`
	Status      map[string]string `toml:"status"`
	Agent       map[string]string `toml:"agent"`
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
	Shape        string `toml:"shape"`
	LeftCap      string `toml:"left_cap"`
	RightCap     string `toml:"right_cap"`
	PaddingLeft  int    `toml:"padding_left"`
	PaddingRight int    `toml:"padding_right"`
}
