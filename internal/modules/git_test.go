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

// TestGitV2BranchParsing verifies porcelain v2 header parsing for branch name,
// ahead, and behind counts. The v2 branch lines are unambiguous regardless of
// branch names that contain "..." or "[" (unlike the v1 ## header).
func TestGitV2BranchParsing(t *testing.T) {
	type tc struct {
		lines  []string
		branch string
		ahead  int
		behind int
	}
	cases := []tc{
		{
			lines:  []string{"# branch.head main", "# branch.ab +2 -1"},
			branch: "main", ahead: 2, behind: 1,
		},
		{
			lines:  []string{"# branch.head feat/foo...bar", "# branch.ab +3 -0"},
			branch: "feat/foo...bar", ahead: 3, behind: 0,
		},
		{
			lines:  []string{"# branch.head feat/foo [wip]", "# branch.ab +0 -4"},
			branch: "feat/foo [wip]", ahead: 0, behind: 4,
		},
		{
			lines:  []string{"# branch.head main"},
			branch: "main", ahead: 0, behind: 0,
		},
		{
			lines:  []string{"# branch.head (detached)"},
			branch: "", ahead: 0, behind: 0,
		},
	}
	for _, c := range cases {
		d := &gitData{}
		for _, line := range c.lines {
			parsePortcelainV2Line(line, d)
		}
		if d.Branch != c.branch {
			t.Errorf("branch = %q, want %q (lines %v)", d.Branch, c.branch, c.lines)
		}
		if d.Ahead != c.ahead {
			t.Errorf("ahead = %d, want %d (lines %v)", d.Ahead, c.ahead, c.lines)
		}
		if d.Behind != c.behind {
			t.Errorf("behind = %d, want %d (lines %v)", d.Behind, c.behind, c.lines)
		}
	}
}

// TestGitV2StatusCounts verifies porcelain v2 status entry parsing.
func TestGitV2StatusCounts(t *testing.T) {
	lines := []string{
		"# branch.head main",
		"1 .M N... 100644 100644 100644 aaa bbb file1.go",     // modified work-tree only
		"1 M. N... 100644 100644 100644 aaa bbb file2.go",     // staged only
		"1 MM N... 100644 100644 100644 aaa bbb file3.go",     // staged + modified
		"2 R. N... 100644 100644 100644 aaa bbb R50 file4.go\tfile4_orig.go", // renamed staged
		"u UU N... 100644 100644 100644 100644 aaa bbb ccc file5.go",        // conflict
		"u AA N... 100644 100644 100644 100644 aaa bbb ccc file6.go",        // conflict
		"? untracked.txt",
		"? another.txt",
	}
	d := &gitData{}
	for _, line := range lines {
		parsePortcelainV2Line(line, d)
	}
	if d.Branch != "main" {
		t.Errorf("branch = %q, want main", d.Branch)
	}
	if d.Staged != 3 { // file2, file3, file4 (renamed staged)
		t.Errorf("staged = %d, want 3", d.Staged)
	}
	if d.Modified != 2 { // file1, file3
		t.Errorf("modified = %d, want 2", d.Modified)
	}
	if d.Conflict != 2 { // file5, file6
		t.Errorf("conflict = %d, want 2", d.Conflict)
	}
	if d.Untracked != 2 {
		t.Errorf("untracked = %d, want 2", d.Untracked)
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
