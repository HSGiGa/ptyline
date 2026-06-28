// Package status holds the normalized StatusState and the module framework. The
// renderer consumes a prepared StatusState only — it never queries git, the
// shell, or modules during rendering. This keeps rendering fast, cacheable, and
// testable (spec §24, ARCHITECTURE.md §2).
package status

import (
	"strconv"
	"time"

	"github.com/hsgiga/ptyline/internal/diagnostics"
)

// maxDurationMS is an upper bound for shell-reported command duration (24 hours).
const maxDurationMS = 86_400_000

// StatusState is the single structured state object read by the renderer. The
// MVP populates a small subset; the remaining fields are reserved so future
// providers (git, agents) slot in without reshaping the type (spec §24.1).
type StatusState struct {
	Terminal               TerminalState
	Shell                  ShellState
	Git                    *GitState // reserved; provider is post-MVP (spec §8.7, §19)
	Modules                ModuleValues
	Agents                 []AgentState // reserved; post-MVP (spec §24.5)
	Diagnostics            diagnostics.Record
	AnimationPhase         int
	ActiveCommandAnimating bool
}

// NewState creates an empty, writable status state.
func NewState() StatusState {
	return StatusState{Modules: make(ModuleValues)}
}

// ApplyShellMeta applies a validated OSC 777 metadata value.
func (s *StatusState) ApplyShellMeta(key, value string) {
	switch key {
	case "cwd":
		s.Shell.CWD = value
	case "command":
		if value != "" {
			s.Shell.ActiveCommand = value
			s.Shell.LastCommand = value
			s.Shell.LastCommandCompleted = false
			s.Shell.LastCommandCompletedAt = time.Time{}
			return
		}
		s.Shell.ActiveCommand = ""
		if s.Shell.LastCommand != "" {
			s.Shell.LastCommandCompleted = true
			s.Shell.LastCommandCompletedAt = time.Now()
		}
	case "exit_code":
		if code, err := strconv.Atoi(value); err == nil && code >= 0 && code <= 255 {
			s.Shell.LastExitCode = code
		}
	case "duration_ms":
		if ms, err := strconv.ParseInt(value, 10, 64); err == nil && ms >= 0 && ms <= maxDurationMS {
			s.Shell.LastDurationMS = int(ms)
		}
	case "ssh_start":
		s.Shell.SSHTarget = value
	case "ssh_end":
		s.Shell.SSHTarget = ""
	}
}

// UpdateModule stores a module's most recent cached snapshot.
func (s *StatusState) UpdateModule(snapshot ModuleSnapshot) {
	if s.Modules == nil {
		s.Modules = make(ModuleValues)
	}
	s.Modules[snapshot.ID] = snapshot
}

// Resize records terminal geometry and current screen mode.
func (s *StatusState) Resize(cols, rows uint16, alternateScreen bool) {
	s.Terminal = TerminalState{Cols: cols, Rows: rows, AlternateScreen: alternateScreen}
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
	CWD                    string
	Username               string
	Hostname               string
	Shell                  string
	ActiveCommand          string
	LastExitCode           int
	LastCommand            string
	LastDurationMS         int
	LastCommandCompleted   bool
	LastCommandCompletedAt time.Time
	SSHTarget              string // set by ssh_start wrapper, cleared by ssh_end
}

// ClearLastCommand resets all "done command" fields after the TTL expires.
func (s *ShellState) ClearLastCommand() {
	s.LastCommand = ""
	s.LastExitCode = 0
	s.LastDurationMS = 0
	s.LastCommandCompleted = false
	s.LastCommandCompletedAt = time.Time{}
}

// GitState is the reserved git provider value (post-MVP, spec §19).
type GitState struct {
	Branch string
	Dirty  bool
}

// ModuleValues maps a module ID to its latest snapshot.
type ModuleValues map[ModuleID]ModuleSnapshot

// AgentStatus is the lifecycle state of an agent (spec §24.5, ARCHITECTURE.md §10).
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
