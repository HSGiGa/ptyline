# 00 — Project Scaffold
Status: [x] done
Depends on: —
Spec refs: spec §21, §22; ARCHITECTURE.md §22

## Goal
A buildable, lint-clean Go project skeleton: module, tooling, CI, the full
`internal/` package tree as compilable stdlib-only stubs, the docs tree, and these
plans. `make build` produces a binary; `make test` is green.

## Deliverables
- `go.mod` (module `github.com/hsgiga/ptyline`, go 1.23), `Makefile`, `.gitignore`,
  `.golangci.yml`.
- `cmd/ptyline/main.go` → `internal/app.Run`.
- `internal/**` stubs with real type/interface signatures and `TODO scaffold (plan NN)`
  bodies.
- `README.md`, `ARCHITECTURE.md`, `docs/**`, `docs/plans/**`.
- `.github/workflows/ci.yml` (build matrix + test + lint).
- Seed unit tests (`internal/reserved`, `internal/app`).

## Approach
1. Create module + tooling.
2. Lay out `internal/` packages so dependencies are acyclic and each compiles
   against the standard library only.
3. Write the docs and plans.

## Invariants
Stubs must compile and `go vet` clean. No external deps yet. Cross-package types
must not form import cycles.

## Acceptance
- [x] `internal/reserved` math + `app` CLI parser implemented with tests.
- [x] `make build` / `make test` / `make vet` succeed.

## Tests
`internal/reserved/area_test.go`, `internal/app/cli_test.go`.

## Out of scope
Any real terminal/PTY behavior — every other package is a stub.
