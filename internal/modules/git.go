package modules

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// Git renders the current branch (and later dirty state). It is the canonical
// "expensive module": it runs on its own interval with a timeout, and the
// renderer reads only the cached snapshot — never shelling out per redraw
// (spec §8.7, plan 15).
type Git struct {
	interval time.Duration
	timeout  time.Duration
	// icon is the (already preset-resolved) glyph shown in front of the branch:
	// the Nerd-Font branch glyph when a Nerd Font is configured, otherwise a
	// plain-font fallback. Empty means no icon.
	icon string
	// cwd reports the directory to run git in (the shell's current dir, tracked
	// via shell-integration). It must be safe to call from the scheduler
	// goroutine. Nil falls back to git's own process cwd.
	cwd func() string
}

// NewGit creates a git module with refresh interval, per-refresh timeout, the
// resolved branch icon (may be empty), and a cwd provider (may be nil).
func NewGit(interval, timeout time.Duration, icon string, cwd func() string) *Git {
	return &Git{interval: interval, timeout: timeout, icon: icon, cwd: cwd}
}

func (m *Git) ID() status.ModuleID     { return "git" }
func (m *Git) Interval() time.Duration { return m.interval }

// Refresh runs `git rev-parse --abbrev-ref HEAD` under ctx's deadline in the
// current directory. Outside a repository (or if git is missing) it yields an
// empty value rather than an error, so the bar simply shows no branch. On timeout
// it returns a stale snapshot rather than blocking the bar.
func (m *Git) Refresh(ctx context.Context) status.ModuleSnapshot {
	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	if m.cwd != nil {
		if dir := m.cwd(); dir != "" {
			args = append([]string{"-C", dir}, args...)
		}
	}
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return status.ModuleSnapshot{
			ID:        m.ID(),
			Value:     status.Text(""),
			Stale:     ctx.Err() != nil,
			Err:       ctx.Err(),
			UpdatedAt: time.Now(),
		}
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return status.ModuleSnapshot{ID: m.ID(), Value: status.Text(""), UpdatedAt: time.Now()}
	}
	text := branch
	if m.icon != "" {
		text = m.icon + " " + branch
	}
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(text),
		UpdatedAt: time.Now(),
	}
}
