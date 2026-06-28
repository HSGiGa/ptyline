# Implementation Plans

The build of ptyline broken into small, independently-executable slices. Each
`NN-*.md` is self-contained and follows a fixed template. Work them roughly in
order; the `Depends on` line is the real constraint.

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
│                                                                └─ 14 diagnostics & replay
15 git module        (after 07/09 — exercises caching)
16 agents stub       (after 07 — reserves types; post-MVP)
```

**Platform scope:** the MVP is **Linux/WSL only** (spec §4). Native macOS and
Windows/ConPTY backends are post-MVP — keep the `darwin`/`windows` build-tagged
stubs compiling but do not let them block MVP plans.

## Milestones

- **After 05** — ptyline is a transparent PTY pass-through (shell runs inside it).
- **After 09** — a real (static) bar is pinned on the last row.
- **After 13** — MVP per spec §18; check against the spec §20 acceptance criteria.

## Conventions

- Stdlib-only until a plan explicitly adds a dependency, then run `make tidy`.
- Every plan that touches `terminal`/`pty`/`proxy` re-reads
  [`../terminal-safety.md`](../terminal-safety.md) first.
- Replace the scaffold `TODO scaffold (plan NN)` markers as you implement; they
  are grep-able anchors.
