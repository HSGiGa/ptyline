package config

import (
	"fmt"
	"regexp"

	"github.com/BurntSushi/toml"
)

// migrateToLatest upgrades raw config bytes from an older schema version to
// CurrentVersion before parsing, so old configs keep working as features are
// added (arch.md §17).
func migrateToLatest(raw []byte) ([]byte, error) {
	var header struct {
		Version *int `toml:"config_version"`
	}
	if _, err := toml.Decode(string(raw), &header); err != nil {
		return nil, err
	}
	if header.Version == nil {
		return nil, fmt.Errorf("config_version is required")
	}
	if *header.Version > CurrentVersion {
		return nil, fmt.Errorf("config_version %d is newer than supported version %d", *header.Version, CurrentVersion)
	}
	if *header.Version < 0 {
		return nil, fmt.Errorf("config_version must not be negative")
	}
	if *header.Version == CurrentVersion {
		return raw, nil
	}
	return regexp.MustCompile(`(?m)^\s*config_version\s*=\s*0\s*(?:#.*)?$`).ReplaceAll(raw, []byte("config_version = 1")), nil
}
