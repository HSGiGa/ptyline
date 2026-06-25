package modules

import (
	"context"
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

// Inside this repository the value carries the branch behind the resolved icon.
// If git is unavailable the value is empty; only the format is asserted.
func TestGitInRepo(t *testing.T) {
	m := NewGit(time.Second, time.Second, "BR", func() string { return "." })
	snap := m.Refresh(context.Background())
	if snap.Value.Text == "" {
		t.Skip("git unavailable or not in a repo")
	}
	if !strings.HasPrefix(snap.Value.Text, "BR ") {
		t.Fatalf("value %q missing the icon prefix", snap.Value.Text)
	}
}

// An empty icon yields just the branch name, no leading space.
func TestGitNoIcon(t *testing.T) {
	m := NewGit(time.Second, time.Second, "", func() string { return "." })
	snap := m.Refresh(context.Background())
	if snap.Value.Text == "" {
		t.Skip("git unavailable or not in a repo")
	}
	if strings.HasPrefix(snap.Value.Text, " ") {
		t.Fatalf("value %q has a leading space with no icon", snap.Value.Text)
	}
}
