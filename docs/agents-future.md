# Agents (Reserved Architecture — Post-MVP)

Source: reserved fields in `internal/status` + future packages. Design: ARCHITECTURE.md
§10–§13. **Not implemented in the MVP** (spec §19) — this doc records the shape so
the MVP avoids choices that would block it.

## Why reserve it now

Agent status (e.g. background coding agents) is the motivating example for several
architectural choices: typed values, priority overflow, multi-line bars, and
multiple data ingestion paths. By reserving `StatusState.Agents`, the
`ModuleValue` types, and `RenderedBar.ClickZones`, agents become an additive
provider+renderer, not a redesign.

## AgentState (ARCHITECTURE.md §10)

```text
AgentState {
  ID, Name, Kind, Status, CWD, Task, Model,
  Tokens, Cost, StartedAt, UpdatedAt, LastEvent, LastTool, Progress
}
AgentStatus ∈ idle|starting|running|waiting|blocked|done|failed|cancelled
```

The UX-critical statuses are `running`, `blocked`, `waiting`, `failed`, `done` —
they tell the user whether an agent is active, needs attention, or is finished.

## Ingestion paths (ARCHITECTURE.md §11)

1. **OSC events** — for tools inside the child PTY; reuse the OSC 777 channel
   (`agent.started` / `agent.update` / `agent.done`). See
   [`shell-integration.md`](shell-integration.md).
2. **Unix socket** — `$XDG_RUNTIME_DIR/ptyline/events.sock`, JSONL events, for
   external runners not tied to the current shell.
3. **Command provider** — periodic `command = "my-agent-status --json"` with a
   timeout, as a simple fallback.

All three normalize into `StatusState.Agents`; the renderer never knows the
source (ARCHITECTURE.md §5).

## Rendering (ARCHITECTURE.md §12)

A separate agent renderer (not mixed into the main bar renderer) with compact
fallbacks driven by priority overflow:

```text
🤖 reviewer running 4m 12k tok · tester blocked permission   (detailed)
🤖 2 agents: reviewer running · tester blocked               (compact)
🤖2                                                          (tiny)
```

## Multi-line / panel mode (ARCHITECTURE.md §13)

Enabled purely by increasing `reserved.Area.Rows` — PTY sizing already derives
from it, so no PTY logic changes:

```text
~/repo main ✓                                      18:42
🤖 reviewer running 4m · tester blocked · writer done
```

## Security (ARCHITECTURE.md §20)

Agent data is **local runtime state, not a trusted command source.** OSC/socket
events only update state; any click action that runs a command is opt-in.
