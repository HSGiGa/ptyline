package config

import (
	"os"
	"path/filepath"
)

// Load reads, migrates, and parses the config. The flow is:
//
//	read file → migrate_to_latest → parse → merge over Default()
//
// If path is empty, DefaultPath() is used; a missing file yields Default()
// without error (spec §13).
//
// TODO scaffold (plan 02): wire TOML parsing (github.com/BurntSushi/toml) and
// field-level merge over Default(); validate enums (anchor, width units, …).
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
		return Config{}, err
	}
	if err := Validate(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// parse decodes already-migrated TOML bytes into a Config layered over Default().
func parse(_ []byte) (Config, error) {
	// TODO scaffold: toml.Unmarshal into Default() and surface the parse error
	// with the file path.
	return Default(), nil
}

// Validate enforces the MVP configuration contract (spec §13.1). Violations are
// startup errors that must name the offending key (the caller adds the file path):
//
//   - config_version is required and must equal CurrentVersion;
//   - unknown top-level/module keys, invalid enums, and invalid width expressions
//     are errors (not silently defaulted);
//   - `bar.format` and `[[bar.block]]` are mutually exclusive;
//   - `bar.height` must be 1 in the normal screen;
//   - module IDs are unique and a block references an enabled module by ID;
//   - custom-command modules must carry a timeout (spec §16, §17).
//
// TODO scaffold (plan 02): implement these checks.
func Validate(_ *Config) error {
	return nil
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
