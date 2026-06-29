# 16 — Agents Architecture Stub
Status: [ ] deferred — future feature; not a readiness blocker
Depends on: 07
Spec refs: ARCHITECTURE.md §10–§12, §11.1, §20; docs/agents-future.md
**Post-MVP. Reserve types and one ingestion path only — do not build full agent support.**

## Goal
Lock in the agent data model and a single ingestion path (OSC) so future agent
work is additive, without shipping a full agent feature.

## Deliverables
- Flesh out `status.AgentState` to the ARCHITECTURE.md §10 shape (status enum, tokens,
  task, timestamps) — types only.
- Extend the OSC parser (plan 06) to recognize `agent.started`/`agent.update`/
  `agent.done` and update `StatusState.Agents`.
- A minimal agent renderer entry that degrades to compact/tiny forms.

## Approach
1. Define `AgentStatus` (idle/starting/running/waiting/blocked/done/failed/cancelled).
2. Parse agent OSC JSON payloads strictly into `AgentState`; update by `ID`.
3. Reuse priority overflow (plan 09) for compact fallbacks (ARCHITECTURE.md §12).

## Invariants
Agent data is local runtime state, not a command source (ARCHITECTURE.md §20). OSC/socket
events only update state. Mouse/click actions stay opt-in.

## Acceptance
- [ ] An `agent.update` OSC event updates `StatusState.Agents` and renders compactly.
- [ ] Agents disabled by default; no MVP behavior changes when unused.

## Tests
Agent OSC parse tests; renderer compact-fallback tests at narrow widths.

## Out of scope
Unix-socket and command providers, multi-line agent panel, full agent UX
(ARCHITECTURE.md §11.2–§13) — later work, unblocked by this stub.
