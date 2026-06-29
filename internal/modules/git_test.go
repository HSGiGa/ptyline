package modules

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// Outside a git repository the module yields empty values (not errors), so the
// bar simply hides git blocks.
func TestGitOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	m := NewGit(time.Second, time.Second, func() string { return dir })
	snap := m.Refresh(context.Background())
	if snap.Value.Text != "" {
		t.Fatalf("outside a repo got %q, want empty", snap.Value.Text)
	}
}

func TestGitBranchUpdatesAfterCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := initGitRepo(t)

	m := NewGit(time.Second, time.Second, func() string { return dir })
	snap := m.Refresh(context.Background())
	if snap.Value.Text != "main" {
		t.Fatalf("initial branch = %q, want %q", snap.Value.Text, "main")
	}

	git(t, dir, "checkout", "-b", "feature")
	snap = m.Refresh(context.Background())
	if snap.Value.Text != "feature" {
		t.Fatalf("checked-out branch = %q, want %q", snap.Value.Text, "feature")
	}
	if snap.Stale || snap.Err != nil {
		t.Fatalf("branch snapshot marked stale/error: %+v", snap)
	}
}

func TestGitTimeoutMarksStale(t *testing.T) {
	fakeGit := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nwhile :; do :; done\n"), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}

	m := NewGit(time.Second, 10*time.Millisecond, func() string { return t.TempDir() })
	m.gitBin = fakeGit
	snap := m.Refresh(context.Background())
	if !snap.Stale {
		t.Fatalf("timeout snapshot not stale: %+v", snap)
	}
	if !errors.Is(snap.Err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v, want deadline exceeded", snap.Err)
	}
	if snap.Value.Text != "" {
		t.Fatalf("timeout value = %q, want empty", snap.Value.Text)
	}
}

// TestGitRefreshAllSubIDs verifies that RefreshAll returns snapshots for all
// expected sub-module IDs in the correct order.
func TestGitRefreshAllSubIDs(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := initGitRepo(t)
	m := NewGit(time.Second, time.Second, func() string { return dir })
	snaps := m.RefreshAll(context.Background())

	want := []status.ModuleID{
		"git", "git.branch",
		"git.staged", "git.modified", "git.untracked", "git.conflict",
		"git.ahead", "git.behind",
		"git.state", "git.dirty",
	}
	if len(snaps) != len(want) {
		t.Fatalf("got %d snapshots, want %d", len(snaps), len(want))
	}
	for i, w := range want {
		if snaps[i].ID != w {
			t.Errorf("snaps[%d].ID = %q, want %q", i, snaps[i].ID, w)
		}
	}
}

// TestGitStatusCounts verifies staged/modified/untracked counts are parsed correctly.
func TestGitStatusCounts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := initGitRepo(t)

	// Add a staged file.
	writeFile(t, dir, "staged.go", "package p\n")
	git(t, dir, "add", "staged.go")

	// Add a modified (unstaged) file.
	writeFile(t, dir, "README.md", "changed\n")

	// Add an untracked file.
	writeFile(t, dir, "untracked.txt", "new\n")

	m := NewGit(time.Second, time.Second, func() string { return dir })
	snaps := snapMap(m.RefreshAll(context.Background()))

	if got := snaps["git.staged"]; got != "1" {
		t.Errorf("git.staged = %q, want %q", got, "1")
	}
	if got := snaps["git.modified"]; got != "1" {
		t.Errorf("git.modified = %q, want %q", got, "1")
	}
	if got := snaps["git.untracked"]; got != "1" {
		t.Errorf("git.untracked = %q, want %q", got, "1")
	}
	if got := snaps["git.dirty"]; got != "*" {
		t.Errorf("git.dirty = %q, want %q", got, "*")
	}
	// No conflicts.
	if got := snaps["git.conflict"]; got != "" {
		t.Errorf("git.conflict = %q, want empty", got)
	}
}

// TestGitCleanRepoDirtyEmpty verifies dirty indicator is empty on a clean repo.
func TestGitCleanRepoDirtyEmpty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := initGitRepo(t)
	m := NewGit(time.Second, time.Second, func() string { return dir })
	snaps := snapMap(m.RefreshAll(context.Background()))

	for _, id := range []string{"git.staged", "git.modified", "git.untracked", "git.conflict", "git.dirty"} {
		if got := snaps[id]; got != "" {
			t.Errorf("%s = %q on clean repo, want empty", id, got)
		}
	}
}

// TestGitAheadBehind verifies ahead/behind parsing from the branch header line.
func TestGitAheadBehind(t *testing.T) {
	cases := []struct {
		line   string
		branch string
		ahead  int
		behind int
	}{
		{"main...origin/main [ahead 2, behind 1]", "main", 2, 1},
		{"main...origin/main [ahead 3]", "main", 3, 0},
		{"main...origin/main [behind 4]", "main", 0, 4},
		{"main...origin/main", "main", 0, 0},
		{"main", "main", 0, 0},
		{"No commits yet on main", "main", 0, 0},
	}
	for _, tc := range cases {
		d := &gitData{}
		parseBranchLine(tc.line, d)
		if d.Branch != tc.branch {
			t.Errorf("parseBranchLine(%q) branch = %q, want %q", tc.line, d.Branch, tc.branch)
		}
		if d.Ahead != tc.ahead {
			t.Errorf("parseBranchLine(%q) ahead = %d, want %d", tc.line, d.Ahead, tc.ahead)
		}
		if d.Behind != tc.behind {
			t.Errorf("parseBranchLine(%q) behind = %d, want %d", tc.line, d.Behind, tc.behind)
		}
	}
}

// TestGitMergeState verifies MERGING state is detected from MERGE_HEAD file.
func TestGitMergeState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := initGitRepo(t)

	// Simulate MERGING state by writing MERGE_HEAD.
	gitDir := filepath.Join(dir, ".git")
	if err := os.WriteFile(filepath.Join(gitDir, "MERGE_HEAD"), []byte("deadbeef\n"), 0o644); err != nil {
		t.Fatalf("write MERGE_HEAD: %v", err)
	}

	m := NewGit(time.Second, time.Second, func() string { return dir })
	snaps := snapMap(m.RefreshAll(context.Background()))

	if got := snaps["git.state"]; got != "MERGING" {
		t.Errorf("git.state = %q, want MERGING", got)
	}
}

// TestGitFormatComposesValue verifies the {git} value is composed from the
// configured format, with `|` conditional separators collapsing empty fields.
func TestGitFormatComposesValue(t *testing.T) {
	cases := []struct {
		name string
		d    gitData
		want string
	}{
		{
			name: "all fields present",
			d:    gitData{Branch: "main", Staged: 2, Modified: 1, Ahead: 3},
			want: "main : * : 3", // {branch} | {dirty} | {ahead}, sep ":"
		},
		{
			name: "clean repo collapses dirty and ahead",
			d:    gitData{Branch: "main"},
			want: "main",
		},
		{
			name: "dirty but not ahead",
			d:    gitData{Branch: "dev", Modified: 1},
			want: "dev : *",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dirty := ""
			if tc.d.Staged+tc.d.Modified+tc.d.Untracked+tc.d.Conflict > 0 {
				dirty = "*"
			}
			got := formatGit(&tc.d, dirty, "{branch} | {dirty} | {ahead}", ":", 0)
			if got != tc.want {
				t.Errorf("formatGit = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGitFormatTruncates verifies the composed value is capped at maxWidth.
func TestGitFormatTruncates(t *testing.T) {
	d := gitData{Branch: "a-very-long-feature-branch-name-that-exceeds"}
	got := formatGit(&d, "", "{branch}", "", 10)
	if w := len([]rune(got)); w > 10 {
		t.Errorf("formatGit width = %d (%q), want <= 10", w, got)
	}
}

// TestGitEmptyFormatKeepsBareBranch verifies that without a format the {git}
// snapshot stays the bare branch (legacy behavior).
func TestGitEmptyFormatKeepsBareBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := initGitRepo(t)
	writeFile(t, dir, "x.txt", "x\n") // make it dirty
	m := NewGit(time.Second, time.Second, func() string { return dir })
	snaps := snapMap(m.RefreshAll(context.Background()))
	if snaps["git"] != "main" {
		t.Errorf("git (no format) = %q, want bare branch %q", snaps["git"], "main")
	}
}

// TestGitWithFormatComposesSnapshot verifies the {git} snapshot reflects the
// format while sub-snapshots stay raw.
func TestGitWithFormatComposesSnapshot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := initGitRepo(t)
	writeFile(t, dir, "x.txt", "x\n") // untracked → dirty
	m := NewGit(time.Second, time.Second, func() string { return dir }).
		WithFormat("{branch} | {dirty}", ":", 0)
	snaps := snapMap(m.RefreshAll(context.Background()))
	if snaps["git"] != "main : *" {
		t.Errorf("git (format) = %q, want %q", snaps["git"], "main : *")
	}
	// Sub-snapshots remain raw regardless of format.
	if snaps["git.branch"] != "main" {
		t.Errorf("git.branch = %q, want main", snaps["git.branch"])
	}
	if snaps["git.dirty"] != "*" {
		t.Errorf("git.dirty = %q, want *", snaps["git.dirty"])
	}
}

// --- helpers ---

func snapMap(snaps []status.ModuleSnapshot) map[string]string {
	m := make(map[string]string, len(snaps))
	for _, s := range snaps {
		m[string(s.ID)] = s.Value.Text
	}
	return m
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "ptyline@example.invalid")
	git(t, dir, "config", "user.name", "ptyline")
	git(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	git(t, dir, "add", "README.md")
	git(t, dir, "commit", "-m", "initial")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
