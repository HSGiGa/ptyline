# Implementation Plans

The build of ptyline broken into small, independently-executable slices. The
current readiness focus is Linux, Linux/WSL, and macOS. Deferred work
(Windows/ConPTY, Agents, diagnostics/replay tooling) is kept as reference only
and does not block readiness.

## Template

```text
# NN — Title
Status: [ ] not started | [~] in progress | [x] done
Depends on: <NN list>
Spec refs: <spec §, ARCHITECTURE.md §>

## Goal          one paragraph: what exists after this slice is done
## Deliverables  files created/changed
## Approach       ordered steps; key types/signatures; packages to add
## Invariants     safety rules this slice must not break (→ docs/terminal-safety.md)
## Acceptance     checklist; map to spec §20 where relevant
## Tests          unit/replay tests to add
## Out of scope   what is deferred to a later NN
```

A plan is **done** only when its Acceptance checklist passes. Update `Status:` and
tick boxes as you go.

## Order & dependency graph

```text
00 scaffold
├─ 01 runtime-env & capabilities
├─ 02 config loader
├─ 03 terminal controller ─┐
├─ 04 pty supervisor ──────┤
│                          └─► 05 event bus & loop ─► 06 ansi/osc filter
│                                                      └─► 07 status state & modules
│                                                           ├─ 08 mvp modules
│                                                           └─► 09 layout & renderer
│                                                                ├─ 10 theme/style/icons
│                                                                ├─ 11 cli
│                                                                ├─ 12 shell-integration adapters
│                                                                ├─ 13 alt-screen & resize
│
15 git module        (after 07/09 — exercises caching)
```

## Deferred reference plans

- `14 diagnostics & replay` — QA/tooling only; not required for product
  readiness.
- `16 agents stub` — future feature; not required for product readiness.

**Platform scope:** readiness targets Linux, Linux/WSL, and macOS. Windows/ConPTY
is deferred future work; keep the Windows build-tagged stubs compiling, but do
not let Windows block current plans.

## Milestones

- **After 05** — ptyline is a transparent PTY pass-through (shell runs inside it).
- **After 09** — a real (static) bar is pinned on the last row.
- **After 13** — terminal-wrapper readiness for Linux/WSL/macOS; check against
  the spec §20 acceptance criteria that apply to Unix PTY targets.

## Conventions

- Stdlib-only until a plan explicitly adds a dependency, then run `make tidy`.
- Every plan that touches `terminal`/`pty`/`proxy` re-reads
  [`../terminal-safety.md`](../terminal-safety.md) first.
- Replace the scaffold `TODO scaffold (plan NN)` markers as you implement; they
  are grep-able anchors.
