package modules

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Outside a git repository the module yields an empty value (not an error bar),
// so the bar simply shows no branch.
func TestGitOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	m := NewGit(time.Second, time.Second, "BR", func() string { return dir })
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

	m := NewGit(time.Second, time.Second, "BR", func() string { return dir })
	snap := m.Refresh(context.Background())
	if snap.Value.Text != "BR main" {
		t.Fatalf("initial branch = %q, want %q", snap.Value.Text, "BR main")
	}

	git(t, dir, "checkout", "-b", "feature")
	snap = m.Refresh(context.Background())
	if snap.Value.Text != "BR feature" {
		t.Fatalf("checked-out branch = %q, want %q", snap.Value.Text, "BR feature")
	}
	if snap.Stale || snap.Err != nil {
		t.Fatalf("branch snapshot marked stale/error: %+v", snap)
	}
}

// An empty icon yields just the branch name, no leading space.
func TestGitNoIcon(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := initGitRepo(t)
	m := NewGit(time.Second, time.Second, "", func() string { return dir })
	snap := m.Refresh(context.Background())
	if snap.Value.Text != "main" {
		t.Fatalf("value = %q, want main", snap.Value.Text)
	}
	if strings.HasPrefix(snap.Value.Text, " ") {
		t.Fatalf("value %q has a leading space with no icon", snap.Value.Text)
	}
}

func TestGitTimeoutMarksStale(t *testing.T) {
	fakeGit := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nwhile :; do :; done\n"), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}

	m := NewGit(time.Second, 10*time.Millisecond, "", func() string { return t.TempDir() })
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

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "ptyline@example.invalid")
	git(t, dir, "config", "user.name", "ptyline")
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
