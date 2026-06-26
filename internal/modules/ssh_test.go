package modules

import (
	"testing"
)

func TestSSHLabelOutsideSSH(t *testing.T) {
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SSH_TTY", "")
	if got := sshLabel(); got != "" {
		t.Fatalf("outside SSH expected empty label, got %q", got)
	}
}

func TestSSHLabelInsideSSH(t *testing.T) {
	t.Setenv("SSH_CLIENT", "192.168.1.1 12345 22")
	t.Setenv("SSH_TTY", "/dev/pts/0")
	t.Setenv("USER", "alice")

	got := sshLabel()
	if got == "" {
		t.Fatal("inside SSH expected non-empty label")
	}
	// Must contain user and a host component separated by @.
	if len(got) < 3 {
		t.Fatalf("label too short: %q", got)
	}
}

func TestSSHLabelStripsHostDomain(t *testing.T) {
	t.Setenv("SSH_CLIENT", "10.0.0.1 54321 22")
	t.Setenv("SSH_TTY", "")
	t.Setenv("USER", "bob")

	got := sshLabel()
	for _, ch := range got {
		if ch == '.' {
			t.Fatalf("label %q contains a dot — domain not stripped", got)
		}
	}
}
