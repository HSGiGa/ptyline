package config

import (
	"bytes"
	"fmt"

	"github.com/BurntSushi/toml"
)

// migrateToLatest upgrades raw config bytes from an older schema version to
// CurrentVersion before parsing, so old configs keep working as features are
// added (ARCHITECTURE.md §17).
//
// Migration pattern for future versions: decode raw into a version-N struct,
// transform fields to the version-(N+1) struct, encode back to TOML. Do NOT
// use regex substitution — it is fragile and skips field-level transformation.
// See the configV0 → Config example below as a blueprint.
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
	// Version 0 → 1: Only the version number changed (no field renames).
	if *header.Version == 0 {
		var v0 configV0
		if _, err := toml.Decode(string(raw), &v0); err != nil {
			return nil, err
		}
		v1 := v0.toV1()
		var buf bytes.Buffer
		if err := toml.NewEncoder(&buf).Encode(v1); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	return raw, nil
}

// configV0 is the v0 schema (identical to Config except config_version = 0).
// It exists as a blueprint for the programmatic migration pattern: when a
// future version 2 adds renamed or restructured fields, model it here and
// implement a toV2() method rather than using regex replacement.
type configV0 = Config

// toV1 converts a v0 config to v1. For v0 the only change is the version
// number; future migrations will transform fields here.
func (c *configV0) toV1() Config {
	out := *c
	out.Version = 1
	return out
}
