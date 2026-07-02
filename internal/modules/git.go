package modules

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/formatting"
	"github.com/hsgiga/ptyline/internal/status/width"
)

// DefaultGitMaxWidth caps the composed {git} value when a format is set so a
// long branch name plus counters cannot overflow the bar.
const DefaultGitMaxWidth = 40

// Git renders git repository state. It is the canonical "expensive module":
// it runs on its own interval with a timeout, and the renderer reads only the
// cached snapshot — never shelling out per redraw (spec §8.7, plan 15).
//
// RefreshAll returns snapshots for {git}, {git.branch}, {git.staged},
// {git.modified}, {git.untracked}, {git.conflict}, {git.ahead}, {git.behind},
// {git.state}, and {git.dirty}. Outside a git repository all values are empty.
type Git struct {
	interval time.Duration
	timeout  time.Duration
	gitBin   string
	// format, when non-empty, composes the {git} value from git fields using
	// short placeholder names ({branch}, {dirty}, {ahead}, …). Empty keeps the
	// legacy behavior: {git} is the bare branch name. The sub-module snapshots
	// (git.branch, git.dirty, …) are always emitted raw regardless of format.
	format    string
	separator string
	maxWidth  int
	// cwd reports the directory to run git in (the shell's current dir, tracked
	// via shell-integration). Must be safe to call from the scheduler goroutine.
	// Nil falls back to git's own process cwd.
	cwd func() string
}

// gitData holds all git status fields parsed in one refresh pass.
type gitData struct {
	Branch    string
	Ahead     int
	Behind    int
	Staged    int
	Modified  int
	Untracked int
	Conflict  int
	State     string // "REBASING", "MERGING", "CHERRY-PICKING", "REVERTING", "BISECTING", or ""
}

// NewGit creates a git module with refresh interval, per-refresh timeout, and a
// cwd provider (may be nil).
func NewGit(interval, timeout time.Duration, cwd func() string) *Git {
	return &Git{interval: interval, timeout: timeout, gitBin: "git", cwd: cwd}
}

// WithFormat sets the composite {git} format and returns the module for
// chaining. An empty format leaves the bare-branch behavior unchanged.
func (m *Git) WithFormat(format, separator string, maxWidth int) *Git {
	m.format = format
	m.separator = separator
	m.maxWidth = maxWidth
	return m
}

func (m *Git) ID() status.ModuleID     { return "git" }
func (m *Git) Interval() time.Duration { return m.interval }

// SameConfig reports whether the module's user-visible config matches the given
// values. Used on reload to decide whether the git goroutine must be restarted.
func (m *Git) SameConfig(interval time.Duration, format, separator string, maxWidth int) bool {
	return m.interval == interval && m.format == format &&
		m.separator == separator && m.maxWidth == maxWidth
}

// Refresh returns the {git} snapshot (branch name). Callers that need all
// sub-module snapshots should call RefreshAll instead.
func (m *Git) Refresh(ctx context.Context) status.ModuleSnapshot {
	return m.RefreshAll(ctx)[0]
}

// RefreshAll runs git status and git rev-parse under ctx's deadline and returns
// snapshots for all git sub-module IDs. The first element is always {git}
// (branch name, for backward compatibility). Outside a repo every value is
// empty. On timeout every snapshot is marked Stale.
func (m *Git) RefreshAll(ctx context.Context) []status.ModuleSnapshot {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.timeout)
		defer cancel()
	}

	data, timedOut := m.collect(ctx)
	now := time.Now()

	intSnap := func(id status.ModuleID, n int) status.ModuleSnapshot {
		v := ""
		if n > 0 {
			v = strconv.Itoa(n)
		}
		return status.ModuleSnapshot{ID: id, Value: status.Text(v), Stale: timedOut, UpdatedAt: now}
	}
	strSnap := func(id status.ModuleID, s string) status.ModuleSnapshot {
		return status.ModuleSnapshot{ID: id, Value: status.Text(s), Stale: timedOut, UpdatedAt: now}
	}

	if data == nil {
		snaps := make([]status.ModuleSnapshot, 0, 10)
		for _, id := range gitSubIDs {
			var ctxErr error
			if timedOut {
				ctxErr = ctx.Err()
			}
			snaps = append(snaps, status.ModuleSnapshot{
				ID: id, Value: status.Text(""), Stale: timedOut, Err: ctxErr, UpdatedAt: now,
			})
		}
		return snaps
	}

	dirtyStr := ""
	if data.Staged+data.Modified+data.Untracked+data.Conflict > 0 {
		dirtyStr = "*"
	}

	// {git} is the bare branch by default, or the composed format when one is set.
	gitValue := data.Branch
	if m.format != "" {
		gitValue = formatGit(data, dirtyStr, m.format, m.separator, m.maxWidth)
	}

	return []status.ModuleSnapshot{
		strSnap("git", gitValue),
		strSnap("git.branch", data.Branch),
		intSnap("git.staged", data.Staged),
		intSnap("git.modified", data.Modified),
		intSnap("git.untracked", data.Untracked),
		intSnap("git.conflict", data.Conflict),
		intSnap("git.ahead", data.Ahead),
		intSnap("git.behind", data.Behind),
		strSnap("git.state", data.State),
		strSnap("git.dirty", dirtyStr),
	}
}

// gitSubIDs is the ordered list of all snapshot IDs RefreshAll returns.
var gitSubIDs = []status.ModuleID{
	"git", "git.branch",
	"git.staged", "git.modified", "git.untracked", "git.conflict",
	"git.ahead", "git.behind",
	"git.state", "git.dirty",
}

// collect runs the git subprocesses and returns parsed data. Returns nil on
// any error (not-a-repo, git missing, etc.); timedOut is true when ctx expired.
//
// Uses --porcelain=v2 --branch (git ≥ 2.11, released 2016): the v2 branch
// header lines are unambiguous and cannot be confused by branch names that
// contain "..." or "[" as in the v1 header.
func (m *Git) collect(ctx context.Context) (data *gitData, timedOut bool) {
	dir := ""
	if m.cwd != nil {
		dir = m.cwd()
	}

	args := m.withDir(dir, "status", "--porcelain=v2", "--branch")
	out, err := exec.CommandContext(ctx, m.gitBin, args...).Output()
	if ctx.Err() != nil {
		return nil, true
	}
	if err != nil {
		return nil, false // not a repo, git missing, or git < 2.11
	}

	d := &gitData{}
	for _, line := range strings.Split(string(out), "\n") {
		parsePortcelainV2Line(line, d)
	}

	// Detect REBASE/MERGE/etc. state from git directory files.
	if gitDir := m.resolveGitDir(ctx, dir); gitDir != "" {
		d.State = detectGitState(gitDir)
	}

	return d, false
}

// parsePortcelainV2Line parses one line from `git status --porcelain=v2 --branch`.
//
// Header lines:
//
//	# branch.head <name>    (or "(detached)")
//	# branch.ab +N -M
//
// Status entries:
//
//	1 XY …  changed (X=index, Y=work-tree; '.' = unmodified)
//	2 XY …  renamed/copied (same XY semantics)
//	u XY …  unmerged
//	? …     untracked
func parsePortcelainV2Line(line string, d *gitData) {
	if len(line) < 2 {
		return
	}
	if line[0] == '#' && len(line) >= 3 && line[1] == ' ' {
		rest := line[2:]
		switch {
		case strings.HasPrefix(rest, "branch.head "):
			name := strings.TrimPrefix(rest, "branch.head ")
			if name != "(detached)" {
				d.Branch = name
			}
		case strings.HasPrefix(rest, "branch.ab "):
			// "+N -M" — ahead and behind counts.
			ab := strings.TrimPrefix(rest, "branch.ab ")
			for _, part := range strings.Fields(ab) {
				if len(part) < 2 {
					continue
				}
				n, err := strconv.Atoi(part[1:])
				if err != nil {
					continue
				}
				switch part[0] {
				case '+':
					d.Ahead = n
				case '-':
					d.Behind = n
				}
			}
		}
		return
	}
	switch line[0] {
	case '1', '2': // changed entry: "1 XY …" or "2 XY …"
		if len(line) < 4 {
			return
		}
		x, y := line[2], line[3]
		if x != '.' {
			d.Staged++
		}
		if y != '.' {
			d.Modified++
		}
	case 'u': // unmerged
		d.Conflict++
	case '?': // untracked
		d.Untracked++
	}
}

// withDir prepends -C dir to git args when dir is non-empty.
func (m *Git) withDir(dir string, args ...string) []string {
	if dir != "" {
		return append([]string{"-C", dir}, args...)
	}
	return args
}

// resolveGitDir runs `git rev-parse --git-dir` to find the .git directory.
// Returns "" on error. The git dir may be relative; we make it absolute.
func (m *Git) resolveGitDir(ctx context.Context, cwd string) string {
	args := m.withDir(cwd, "rev-parse", "--git-dir")
	out, err := exec.CommandContext(ctx, m.gitBin, args...).Output()
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) && cwd != "" {
		gitDir = filepath.Join(cwd, gitDir)
	}
	return gitDir
}

// detectGitState inspects special files inside the git directory to determine
// the current repository operation state.
func detectGitState(gitDir string) string {
	switch {
	case fileExists(filepath.Join(gitDir, "MERGE_HEAD")):
		return "MERGING"
	case dirExists(filepath.Join(gitDir, "rebase-merge")):
		return "REBASING"
	case dirExists(filepath.Join(gitDir, "rebase-apply")):
		return "REBASING"
	case fileExists(filepath.Join(gitDir, "CHERRY_PICK_HEAD")):
		return "CHERRY-PICKING"
	case fileExists(filepath.Join(gitDir, "REVERT_HEAD")):
		return "REVERTING"
	case fileExists(filepath.Join(gitDir, "BISECT_LOG")):
		return "BISECTING"
	}
	return ""
}

// formatGit composes the {git} value from a format template using short field
// names ({branch}, {dirty}, {staged}, {modified}, {untracked}, {conflict},
// {ahead}, {behind}, {state}). Numeric fields are empty when zero so `|`
// conditional separators collapse around them (same convention as the command
// module). The result is truncated to maxWidth.
func formatGit(d *gitData, dirty, format, separator string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = DefaultGitMaxWidth
	}
	num := func(n int) string {
		if n > 0 {
			return strconv.Itoa(n)
		}
		return ""
	}
	replacer := strings.NewReplacer(
		"{branch}", d.Branch,
		"{dirty}", dirty,
		"{staged}", num(d.Staged),
		"{modified}", num(d.Modified),
		"{untracked}", num(d.Untracked),
		"{conflict}", num(d.Conflict),
		"{ahead}", num(d.Ahead),
		"{behind}", num(d.Behind),
		"{state}", d.State,
	)
	text := formatting.CollapseSeparators(replacer.Replace(format), separator)
	return width.Truncate(text, maxWidth, "right")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
