package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureUserConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	want := filepath.Join(dir, "ptyline", "config.toml")

	created, path, err := EnsureUserConfig()
	if err != nil {
		t.Fatalf("EnsureUserConfig: %v", err)
	}
	if !created {
		t.Fatal("want created=true on first run")
	}
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	// The seeded file must be a valid config that loads cleanly.
	if _, _, err := Load(""); err != nil {
		t.Fatalf("Load seeded config: %v", err)
	}

	// A second call must not touch an existing config, even a user-edited one.
	const edited = "config_version = 1\n"
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	created, _, err = EnsureUserConfig()
	if err != nil {
		t.Fatalf("EnsureUserConfig (second call): %v", err)
	}
	if created {
		t.Fatal("want created=false when config already exists")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != edited {
		t.Fatalf("existing config was overwritten: %q", raw)
	}
}
