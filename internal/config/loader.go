package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Load reads, migrates, and parses the config. The flow is:
//
//	read file → migrate_to_latest → parse → merge over Default()
//
// If path is empty, DefaultPath() is used; a missing file yields Default()
// without error (spec §13).
func Load(path string) (Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, err
	}
	migrated, err := migrateToLatest(raw)
	if err != nil {
		return Config{}, err
	}
	cfg, err := parse(migrated)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := Validate(&cfg); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// parse decodes already-migrated TOML bytes into a Config layered over Default().
func parse(raw []byte) (Config, error) {
	cfg := Default()
	metadata, err := toml.Decode(string(raw), &cfg)
	if err != nil {
		return Config{}, err
	}
	if !metadata.IsDefined("config_version") {
		return Config{}, fmt.Errorf("config_version is required")
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("unknown key %q", undecoded[0])
	}
	// A user who specifies a single-line `format` (or `[[bar.block]]`) but no
	// `[[bar.row]]` overrides the multi-line default; otherwise the default rows
	// would shadow their intent (defaults fill unset fields).
	if !metadata.IsDefined("bar", "row") &&
		(metadata.IsDefined("bar", "format") || metadata.IsDefined("bar", "block")) {
		cfg.Bar.Rows = nil
	}
	return cfg, nil
}

// Validate enforces the MVP configuration contract (spec §13.1). Violations are
// startup errors that must name the offending key (the caller adds the file path):
//
//   - config_version is required and must equal CurrentVersion;
//   - unknown top-level/module keys, invalid enums, and invalid width expressions
//     are errors (not silently defaulted);
//   - `bar.format` and `[[bar.block]]` are mutually exclusive;
//   - the reserved height is the number of `[[bar.row]]` entries (or 1 for the
//     single-line `bar.format` fallback) and must be >= 1;
//   - module IDs are unique and a block references an enabled module by ID;
//   - custom-command modules must carry a timeout (spec §16, §17).
func Validate(cfg *Config) error {
	if cfg.Version != CurrentVersion {
		return fmt.Errorf("config_version must be %d", CurrentVersion)
	}
	// Multi-line: the reserved height follows the [[bar.row]] count when present.
	if len(cfg.Bar.Rows) > 0 {
		cfg.Bar.Height = uint16(len(cfg.Bar.Rows))
	}
	if cfg.Bar.Height < 1 {
		return fmt.Errorf("bar.height must be >= 1")
	}
	if cfg.Bar.Format != "" && len(cfg.Bar.Blocks) > 0 {
		return fmt.Errorf("bar.format and bar.block are mutually exclusive")
	}
	if !oneOf(cfg.Bar.Mode, "single-line", "agent-panel") {
		return fmt.Errorf("bar.mode has invalid value %q", cfg.Bar.Mode)
	}
	if !oneOf(cfg.Icons.Preset, "nerd-font", "emoji", "ascii") {
		return fmt.Errorf("icons.preset has invalid value %q", cfg.Icons.Preset)
	}
	if !oneOf(cfg.Icons.EmojiWidth, "auto", "1", "2") {
		return fmt.Errorf("icons.emoji_width has invalid value %q", cfg.Icons.EmojiWidth)
	}
	for index, block := range cfg.Bar.Blocks {
		prefix := fmt.Sprintf("bar.block[%d]", index)
		if !oneOf(block.Anchor, "left", "center", "right") {
			return fmt.Errorf("%s.anchor has invalid value %q", prefix, block.Anchor)
		}
		if !oneOf(block.Align, "left", "center", "right") {
			return fmt.Errorf("%s.align has invalid value %q", prefix, block.Align)
		}
		if !oneOf(block.Truncate, "left", "right", "middle", "none") {
			return fmt.Errorf("%s.truncate has invalid value %q", prefix, block.Truncate)
		}
		for name, width := range map[string]string{"width": block.Width, "min_width": block.MinWidth, "max_width": block.MaxWidth} {
			if width != "" && !validWidth(width) {
				return fmt.Errorf("%s.%s has invalid value %q", prefix, name, width)
			}
		}
		module, ok := cfg.Modules[block.Module]
		if !ok || !module.Enabled {
			return fmt.Errorf("%s.module references unavailable module %q", prefix, block.Module)
		}
	}
	for id, module := range cfg.Modules {
		if module.Command != "" && module.TimeoutMS <= 0 {
			return fmt.Errorf("module.%s.timeout_ms is required for custom commands", id)
		}
	}
	return nil
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

var numericWidth = regexp.MustCompile(`^[1-9][0-9]*%?$`)

func validWidth(width string) bool {
	if width == "auto" || width == "fill" {
		return true
	}
	if !numericWidth.MatchString(width) {
		return false
	}
	if !strings.HasSuffix(width, "%") {
		return true
	}
	percent, _ := strconv.Atoi(strings.TrimSuffix(width, "%"))
	return percent <= 100
}

// DefaultPath returns $XDG_CONFIG_HOME/ptyline/config.toml, falling back to
// ~/.config/ptyline/config.toml (spec §13).
func DefaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ptyline", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "ptyline.toml"
	}
	return filepath.Join(home, ".config", "ptyline", "config.toml")
}

// FindProjectConfig returns the closest .ptyline file at or above dir. Project
// configuration is deliberately opt-in: callers decide which fields to apply.
func FindProjectConfig(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(dir, ".ptyline")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
