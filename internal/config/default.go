package config

// Default returns the built-in configuration used when no config file exists or
// to fill unset fields. It must be readable without Nerd Fonts or emoji (spec §20).
func Default() Config {
	return Config{
		Version:           CurrentVersion,
		Shell:             "auto",
		RefreshIntervalMS: 1000,
		Bar: BarConfig{
			// Height is advisory; the reserved row count is the number of [[bar.row]]
			// entries (or 1 for the single-line Format fallback). Validate derives it.
			Height: 1,
			Mode:   "single-line",
			// Two-row default: a dashes "border" top line carrying git in the
			// middle slot (left/right slots empty but available), and the main
			// content line. Reserved rows = len(Rows). Format stays as the
			// single-line fallback when Rows is empty.
			Rows: []RowConfig{
				// Fill is the box-drawing horizontal "─" (U+2500), which joins into a
				// solid rule; a plain "-" looks like a dashed line. Set fill = "-" for
				// an ASCII-only fallback.
				{Format: "|| {git} ||", Fill: "─"},
				{Format: "{hostname} {cwd} || {time}"},
			},
			Format:    "{hostname} {cwd} || {time}",
			Separator: " | ",
		},
		Theme: ThemeConfig{
			ColorScheme: "default",
			Style:       "flat",
			Icons:       "ascii",
			Fallback:    "ascii",
		},
		Icons: IconsConfig{
			Enabled:    true,
			Preset:     "ascii",
			EmojiWidth: "auto",
			Fallback:   true,
		},
		Modules: map[string]ModuleConfig{
			"time":     {Enabled: true, Format: "%H:%M:%S", IntervalMS: 1000},
			"hostname": {Enabled: true},
			"cwd":      {Enabled: true, Mode: "shell-integration"},
		},
	}
}
