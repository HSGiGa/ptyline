// Package status holds the normalized StatusState and the module framework. The
// renderer consumes a prepared StatusState only — it never queries git, the
// shell, or modules during rendering. This keeps rendering fast, cacheable, and
// testable (spec §24, arch.md §2).
package status

import (
	"time"

	"github.com/hsgiga/ptyline/internal/diagnostics"
)

// StatusState is the single structured state object read by the renderer. The
// MVP populates a small subset; the remaining fields are reserved so future
// providers (git, agents) slot in without reshaping the type (spec §24.1).
type StatusState struct {
	Terminal    TerminalState
	Shell       ShellState
	Git         *GitState // reserved; provider is post-MVP (spec §8.7, §19)
	Modules     ModuleValues
	Agents      []AgentState // reserved; post-MVP (spec §24.5)
	Diagnostics diagnostics.Record
}

// TerminalState mirrors the current real-terminal geometry and screen mode.
type TerminalState struct {
	Cols            uint16
	Rows            uint16
	AlternateScreen bool
}

// ShellState is populated from shell-integration OSC messages (spec §9). It is
// optional: the wrapper works with any shell or command without an adapter.
type ShellState struct {
	CWD            string
	Username       string
	Hostname       string
	Shell          string
	LastExitCode   int
	LastCommand    string
	LastDurationMS int
}

// GitState is the reserved git provider value (post-MVP, spec §19).
type GitState struct {
	Branch string
	Dirty  bool
}

// ModuleValues maps a module ID to its latest snapshot.
type ModuleValues map[ModuleID]ModuleSnapshot

// AgentStatus is the lifecycle state of an agent (spec §24.5, arch.md §10).
type AgentStatus string

const (
	AgentIdle      AgentStatus = "idle"
	AgentStarting  AgentStatus = "starting"
	AgentRunning   AgentStatus = "running"
	AgentWaiting   AgentStatus = "waiting"
	AgentBlocked   AgentStatus = "blocked"
	AgentDone      AgentStatus = "done"
	AgentFailed    AgentStatus = "failed"
	AgentCancelled AgentStatus = "cancelled"
)

// AgentState is the reserved first-class agent type (spec §24.5). Unused by the
// MVP; present so the architecture does not change to add agents.
type AgentState struct {
	ID        string
	Name      string
	Status    AgentStatus
	CWD       string
	Task      string
	Tokens    *uint64
	Cost      *float64
	UpdatedAt time.Time
}
