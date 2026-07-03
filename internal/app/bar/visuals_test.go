package bar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/status/style"
	"github.com/hsgiga/ptyline/internal/status/theme"
)

func TestVisualsFromConfigAppliesInlinePaletteAndStyles(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.Palette = map[string]string{
		"accent": "#112233",
		"brand":  "accent",
	}
	cfg.Theme.Status = map[string]string{"ok": "green"}
	cfg.Styles = map[string]config.StyleConfig{
		"time": {
			FG:           "brand",
			BG:           "muted", // muted=brightblack is in the default palette
			Bold:         true,
			PaddingLeft:  1,
			PaddingRight: 1,
		},
	}

	// Empty config dir: the default color_scheme/style resolve to files that are
	// not installed, so rendering falls back to the terminal-native palette.
	configPath := filepath.Join(t.TempDir(), "config.toml")
	visuals, err := VisualsFromConfig(cfg, theme.TrueColor, configPath, "bash")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := visuals.Theme.FG("brand"), "\x1b[38;2;17;34;51m"; got != want {
		t.Fatalf("brand fg = %q, want %q", got, want)
	}
	if got, want := visuals.Theme.FG("ok"), "\x1b[38;2;0;205;0m"; got != want {
		t.Fatalf("ok fg = %q, want %q", got, want)
	}
	got := visuals.Styles["time"]
	if got.FG != "brand" || got.BG != "muted" || !got.Bold || got.PaddingLeft != 1 || got.PaddingRight != 1 {
		t.Fatalf("time style = %+v", got)
	}
}

func TestVisualsFromConfigLoadsThemeFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	writeThemeFile(t, dir, "custom.toml", `
name = "custom"

[palette]
accent = "#abcdef"
panel = "accent"

[style.time]
fg = "muted"
bg = "panel"
bold = true
padding_left = 1
padding_right = 1
`)
	cfg := config.Default()
	cfg.Theme.ColorScheme = "custom"

	visuals, err := VisualsFromConfig(cfg, theme.TrueColor, configPath, "bash")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := visuals.Theme.BG("panel"), "\x1b[48;2;171;205;239m"; got != want {
		t.Fatalf("panel bg = %q, want %q", got, want)
	}
	if got := visuals.Styles["time"]; got.FG != "muted" || got.BG != "panel" || !got.Bold {
		t.Fatalf("file time style = %+v", got)
	}
}

func TestVisualsFromConfigRejectsMissingThemeFile(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.ColorScheme = "missing"

	_, err := VisualsFromConfig(cfg, theme.TrueColor, filepath.Join(t.TempDir(), "config.toml"), "bash")
	if err == nil || !strings.Contains(err.Error(), "theme \"missing\" not found") {
		t.Fatalf("VisualsFromConfig error = %v, want missing theme", err)
	}
}

func TestVisualsFromConfigRejectsInvalidStyleColor(t *testing.T) {
	cfg := config.Default()
	cfg.Styles = map[string]config.StyleConfig{
		"time": {FG: "not-a-color"},
	}

	configPath := filepath.Join(t.TempDir(), "config.toml")
	_, err := VisualsFromConfig(cfg, theme.TrueColor, configPath, "bash")
	if err == nil || !strings.Contains(err.Error(), "style.time.fg") {
		t.Fatalf("VisualsFromConfig error = %v, want style.time.fg", err)
	}
}

// TestVisualsFromConfigResolvesDefaultPerShell verifies color_scheme = "default"
// and style = "default" pick the shell's palette and style preset: fish/bash use
// the flat preset, zsh the powerline preset.
func TestVisualsFromConfigResolvesDefaultPerShell(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	writeThemeFile(t, dir, "fish-default.toml", "name=\"fish-default\"\n[palette]\naccent=\"#010203\"\n")
	writeThemeFile(t, dir, "bash-default.toml", "name=\"bash-default\"\n[palette]\naccent=\"#161718\"\n")
	writeThemeFile(t, dir, "zsh-default.toml",
		"name=\"zsh-default\"\n[palette]\n\"base.bg\"=\"#000000\"\naccent=\"#040506\"\n")
	writeStyleFile(t, dir, "flat.toml", "[style.hostname]\nfg=\"accent\"\n")
	writeStyleFile(t, dir, "powerline.toml", "[style.hostname]\nfg=\"base.bg\"\nbg=\"accent\"\nshape=\"powerline\"\n")

	cases := map[string]struct {
		accent string
		shape  style.Shape
	}{
		"fish": {"\x1b[38;2;1;2;3m", style.ShapeFlat},
		"bash": {"\x1b[38;2;22;23;24m", style.ShapeFlat},
		"zsh":  {"\x1b[38;2;4;5;6m", style.ShapePowerline},
	}
	for shell, want := range cases {
		visuals, err := VisualsFromConfig(config.Default(), theme.TrueColor, configPath, shell)
		if err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		if got := visuals.Theme.FG("accent"); got != want.accent {
			t.Errorf("%s accent fg = %q, want %q", shell, got, want.accent)
		}
		if got := visuals.Styles["hostname"].Shape; got != want.shape {
			t.Errorf("%s hostname shape = %q, want %q", shell, got, want.shape)
		}
	}
}

// TestVisualsFromConfigDefaultUsesBuiltinTheme verifies that with no theme/style
// files on disk the default resolution still succeeds by using the shipped
// (embedded) shell-default theme, and that the embedded copy matches the
// on-disk one. This is the fresh-install path: it must not fall back to the
// terminal-native palette now that the built-ins are always present.
func TestVisualsFromConfigDefaultUsesBuiltinTheme(t *testing.T) {
	emptyDir := filepath.Join(t.TempDir(), "config.toml")
	fromEmbed, err := VisualsFromConfig(config.Default(), theme.TrueColor, emptyDir, "zsh")
	if err != nil {
		t.Fatalf("embed resolution: %v", err)
	}
	fromDisk, err := VisualsFromConfig(config.Default(), theme.TrueColor, repoConfigPath(), "zsh")
	if err != nil {
		t.Fatalf("disk resolution: %v", err)
	}
	if got, want := fromEmbed.Theme.FG("ok"), fromDisk.Theme.FG("ok"); got != want {
		t.Fatalf("built-in theme fg = %q, want %q (matching on-disk zsh-default)", got, want)
	}
	if native := "\x1b[38;2;0;205;0m"; fromEmbed.Theme.FG("ok") == native {
		t.Fatal("default resolution unexpectedly fell back to native palette")
	}
}

func writeThemeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	writeUnder(t, dir, "themes", name, body)
}

func writeStyleFile(t *testing.T, dir, name, body string) {
	t.Helper()
	writeUnder(t, dir, "styles", name, body)
}

func writeUnder(t *testing.T, dir, sub, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sub, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
