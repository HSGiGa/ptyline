# 02 — Config Loader
Status: [x] done
Depends on: 00
Spec refs: spec §13, §13.1; ARCHITECTURE.md §17

## Goal
TOML config loads from the XDG path (or `--config`), missing files fall back to
defaults without error, the schema version is migrated forward, and values layer
over `config.Default()`.

## Deliverables
- `internal/config/loader.go` — real `parse` via `github.com/BurntSushi/toml`.
- `internal/config/migrate.go` — read `config_version`, apply ordered steps.
- Validation of enums (anchor, align, width units, truncate, icon preset).

## Approach
1. `make tidy` after adding the toml dependency.
2. Unmarshal into a `Config` pre-seeded from `Default()` so unset fields keep
   defaults; or unmarshal then merge — pick one and document it.
3. `migrateToLatest`: parse just `config_version`, run `v(n)→v(n+1)` steps to
   `CurrentVersion`, return upgraded bytes.
4. Validate and return clear errors (spec §15 "invalid config").

## Invariants (spec §13.1)
A missing file is not an error. `config_version` is **required** and must equal 1.
Unknown top-level/module keys, invalid enums, and invalid width expressions are
**startup errors naming the file+key** (not silently defaulted). `bar.format` and
`[[bar.block]]` are mutually exclusive. `bar.height` must be 1. Module IDs unique.
Custom-command modules must carry a timeout (spec §16, §17).

## Acceptance
- [x] Loads the spec §13 example; missing `config_version` and unknown keys are
  startup errors that name the key.
- [x] `bar.format` + `[[bar.block]]` together is rejected.
- [x] `config_version` migration round-trips; enum/width validation rejects bad input.

## Tests
Golden-file tests for the spec §13 examples; migration from a v0 fixture; invalid
config cases.

## Out of scope
Applying config to layout/theme (plans 09/10). Hot reload.
