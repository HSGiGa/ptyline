package bar

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/status/theme"
)

// repoConfigPath points VisualsFromConfig at the repository's config/ directory
// so theme and style lookups resolve config/themes and config/styles.
func repoConfigPath() string {
	return filepath.Join("..", "..", "..", "config", "config.toml")
}

func listTOMLNames(t *testing.T, sub string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("..", "..", "..", "config", sub, "*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatalf("no %s/*.toml found", sub)
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, strings.TrimSuffix(filepath.Base(m), ".toml"))
	}
	return names
}

// TestShippedThemesAndStylesLoad renders every shipped color scheme against every
// shipped style preset, so a malformed palette, dangling color reference, or bad
// shape in the TOML fails here rather than at a user's terminal.
func TestShippedThemesAndStylesLoad(t *testing.T) {
	configPath := repoConfigPath()
	themes := listTOMLNames(t, "themes")
	styles := listTOMLNames(t, "styles")

	for _, scheme := range themes {
		for _, styleName := range styles {
			cfg := config.Default()
			cfg.Theme.ColorScheme = scheme
			cfg.Theme.Style = styleName
			if _, err := VisualsFromConfig(cfg, theme.TrueColor, configPath, "bash"); err != nil {
				t.Errorf("color_scheme=%q style=%q: %v", scheme, styleName, err)
			}
		}
	}
}

// TestShippedDefaultsResolvePerShell renders the built-in default resolution for
// each supported shell using the shipped files.
func TestShippedDefaultsResolvePerShell(t *testing.T) {
	configPath := repoConfigPath()
	for _, shell := range []string{"fish", "zsh", "bash", "sh", ""} {
		if _, err := VisualsFromConfig(config.Default(), theme.TrueColor, configPath, shell); err != nil {
			t.Errorf("shell %q: %v", shell, err)
		}
	}
}
