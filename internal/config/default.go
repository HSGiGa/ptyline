package config

// Default returns the built-in configuration used when no config file exists or
// to fill unset fields. It must be readable without Nerd Fonts or emoji (spec §20).
func Default() Config {
	return Config{
		Version:           CurrentVersion,
		Shell:             "auto",
		RefreshIntervalMS: 1000,
		Bar: BarConfig{
			// Built-in fallback is intentionally minimal: a rule-like stripe and a
			// clock. Rich development layouts live in config/config.toml and are
			// passed via --config by `make run`.
			Rows: []RowConfig{
				// Fill is the box-drawing horizontal "─" (U+2500), which joins into a
				// solid rule; a plain "-" looks like a dashed line. Set fill = "-" for
				// an ASCII-only fallback.
				{Format: "", Fill: "─"},
				{Format: "|| {time}"},
			},
			Format:    "|| {time}",
			Justify:   "center",
			Separator: " | ",
		},
		Theme: ThemeConfig{
			ColorScheme: "default",
			Style:       "default",
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
			"time": {
				Enabled:    true,
				Format:     "%H:%M:%S",
				IntervalMS: 1000,
			},
			"user": {
				Enabled: true,
			},
			"runtime": {
				Enabled: true,
			},
			"shell": {
				Enabled: true,
			},
			"env": {
				Enabled: true,
				Env:     []string{},
			},
			"load": {
				Enabled:    false,
				IntervalMS: 5000,
				TimeoutMS:  100,
			},
			"cpu": {
				Enabled:    false,
				IntervalMS: 2000,
				TimeoutMS:  100,
			},
			"memory": {
				Enabled:    false,
				IntervalMS: 5000,
				TimeoutMS:  100,
			},
			"battery": {
				Enabled:    false,
				IntervalMS: 30000,
				TimeoutMS:  250,
			},
			"disk": {
				Enabled:    false,
				IntervalMS: 60000,
				TimeoutMS:  250,
			},
		},
	}
}
