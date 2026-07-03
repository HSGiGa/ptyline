package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTempBin(t *testing.T, dir string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, "testbin")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatalf("write temp bin: %v", err)
	}
	return path
}

func TestBinaryIdentityNotChanged(t *testing.T) {
	dir := t.TempDir()
	path := writeTempBin(t, dir, []byte("fakebinary"))

	info, _ := os.Stat(path)
	b := binaryIdentity{path: path, modTime: info.ModTime(), size: info.Size(), ok: true}

	changed, err := b.changed()
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if changed {
		t.Fatal("want not changed, got changed")
	}
}

func TestBinaryIdentityChangedOnRewrite(t *testing.T) {
	dir := t.TempDir()
	path := writeTempBin(t, dir, []byte("fakebinary"))

	info, _ := os.Stat(path)
	b := binaryIdentity{path: path, modTime: info.ModTime(), size: info.Size(), ok: true}

	// Rewrite with different content to change mtime and size.
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("fakebinary-v2"), 0o755); err != nil {
		t.Fatal(err)
	}

	changed, err := b.changed()
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if !changed {
		t.Fatal("want changed, got not changed")
	}
}

func TestBinaryIdentityDeletedFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempBin(t, dir, []byte("fakebinary"))

	info, _ := os.Stat(path)
	b := binaryIdentity{path: path, modTime: info.ModTime(), size: info.Size(), ok: true}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	changed, err := b.changed()
	if err == nil {
		t.Fatal("want error for deleted binary, got nil")
	}
	if changed {
		t.Fatal("want not changed on error, got changed")
	}
}

func TestBinaryIdentityNotExecutable(t *testing.T) {
	dir := t.TempDir()
	path := writeTempBin(t, dir, []byte("fakebinary"))

	info, _ := os.Stat(path)
	b := binaryIdentity{path: path, modTime: info.ModTime(), size: info.Size(), ok: true}

	// Remove executable bit and touch to change mtime.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("fakebinary-noexec"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := b.changed()
	if err == nil {
		t.Fatal("want error for non-executable binary, got nil")
	}
	if changed {
		t.Fatal("want not changed when binary is not executable")
	}
}

func TestBinaryIdentityOkFalse(t *testing.T) {
	b := binaryIdentity{ok: false}
	changed, err := b.changed()
	if err != nil {
		t.Fatalf("want nil err for ok=false, got %v", err)
	}
	if changed {
		t.Fatal("want not changed for ok=false")
	}
}
