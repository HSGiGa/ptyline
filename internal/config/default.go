package config

// Default returns the built-in configuration used when no config file exists or
// to fill unset fields. It must be readable without Nerd Fonts or emoji (spec §20).
func Default() Config {
	return Config{
		Version:           CurrentVersion,
		Shell:             "auto",
		RefreshIntervalMS: 1000,
		Bar: BarConfig{
			// height must be 1 in the normal screen; alternate-screen behavior is
			// fixed (bar hidden) and not configurable in the MVP (spec §13.1, §11).
			Height: 1,
			Mode:   "single-line",
			// MVP default uses only initial modules — git is a post-MVP provider
			// (spec §8.7). Center section is empty (no `||` ... `||`).
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
