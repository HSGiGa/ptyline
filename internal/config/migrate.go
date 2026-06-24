package config

// migrateToLatest upgrades raw config bytes from an older schema version to
// CurrentVersion before parsing, so old configs keep working as features are
// added (arch.md §17).
//
// TODO scaffold (plan 02): read `config_version` from the raw TOML, apply
// ordered migration steps v(n) → v(n+1), and return the upgraded bytes.
func migrateToLatest(raw []byte) ([]byte, error) {
	return raw, nil
}
