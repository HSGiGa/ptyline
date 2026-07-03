package bar

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/status/theme"
)

// TestBuiltinThemesResolveWithoutDiskFiles renders a shipped scheme/style when
// the user's config directory has no themes/ or styles/ files, exercising the
// embedded fallback that lets a fresh install work without `make install-config`.
func TestBuiltinThemesResolveWithoutDiskFiles(t *testing.T) {
	emptyConfig := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.Default()
	cfg.Theme.ColorScheme = "nord"
	cfg.Theme.Style = "powerline"
	if _, err := VisualsFromConfig(cfg, theme.TrueColor, emptyConfig, "bash"); err != nil {
		t.Fatalf("built-in theme should resolve from embed: %v", err)
	}
}

// TestDiskThemeOverridesBuiltin proves the on-disk copy wins over the embedded
// one: a malformed themes/nord.toml must be parsed (and fail) rather than
// silently falling back to the built-in nord.
func TestDiskThemeOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "themes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "themes", "nord.toml"), []byte("this = "), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Theme.ColorScheme = "nord"
	cfg.Theme.Style = "flat"
	if _, err := VisualsFromConfig(cfg, theme.TrueColor, filepath.Join(dir, "config.toml"), "bash"); err == nil {
		t.Fatal("expected malformed on-disk theme to be used and fail")
	}
}
