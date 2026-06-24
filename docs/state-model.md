# State Model

Source: `internal/status/state.go`, `internal/status/module.go`. Design: spec
§8.5, §24.1, §24.3.

## `StatusState` — the single read model

The renderer consumes a prepared `StatusState` and nothing else. It is updated
only by the event loop, from module snapshots, shell-integration messages, and
lifecycle/resize events.

```text
StatusState
├─ Terminal:    { Cols, Rows, AlternateScreen }
├─ Shell:       { CWD, Username, Hostname, Shell, LastExitCode, LastCommand, LastDurationMS }
├─ Git:         *GitState         (reserved; provider is post-MVP)
├─ Modules:     map[ModuleID]ModuleSnapshot
├─ Agents:      []AgentState      (reserved; post-MVP — spec §24.5)
└─ Diagnostics: diagnostics.Record
```

The MVP only fills `Terminal`, parts of `Shell`, and a few `Modules`. The other
fields exist now so adding features later does not reshape the type or the
renderer's contract.

## Typed module values (spec §24.3)

Modules return a typed `ModuleValue` **struct** (a `Kind` discriminator selects
the active field), never a bare string, so the renderer can format numbers,
distinguish errors, and show staleness:

```go
type ModuleValue struct {
    Kind   ModuleValueKind // Text | Number | Bool | Status | JSON
    Text   string
    Number float64
    Bool   bool
    Status *StatusValue    // { Level: ok|warn|error, Text }
    JSON   json.RawMessage
}
```

Use the constructors `status.Text/Number/Bool/Status(...)`. `StatusValue.Level`
maps to a **theme token**, not a raw color — see
[`layout-and-rendering.md`](layout-and-rendering.md) and spec §24.4.

## Snapshots and freshness

```go
type ModuleSnapshot struct {
    ID        ModuleID
    Value     ModuleValue
    UpdatedAt time.Time
    Stale     bool
    Err       error
}
```

- **Stale** — the provider timed out or hasn't refreshed; the renderer may dim it.
- **Err** — the provider failed; the renderer may hide the module or show a
  warning indicator (spec §24.3).

This lets "git timed out", "agent socket stale", and "battery unavailable" be
represented cleanly instead of being smuggled inside a string.

## Caching discipline (spec §8.7)

Expensive modules (git, custom commands) refresh on their **own interval** with a
**timeout**, writing a new snapshot. The renderer always reads the cached
snapshot. Never shell out per redraw. Defaults: status 1000ms, git 2000ms, custom
command timeout 200ms (spec §16).
