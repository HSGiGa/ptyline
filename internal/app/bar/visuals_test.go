package bar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hsgiga/ptyline/internal/config"
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
			BG:           "base.bg",
			Bold:         true,
			PaddingLeft:  1,
			PaddingRight: 1,
		},
	}

	visuals, err := VisualsFromConfig(cfg, theme.TrueColor, "")
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
	if got.FG != "brand" || got.BG != "base.bg" || !got.Bold || got.PaddingLeft != 1 || got.PaddingRight != 1 {
		t.Fatalf("time style = %+v", got)
	}
}

func TestVisualsFromConfigLoadsThemeFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(filepath.Join(dir, "themes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "themes", "custom.toml"), []byte(`
name = "custom"

[palette]
accent = "#abcdef"
panel = "accent"

[style.time]
fg = "base.bg"
bg = "panel"
bold = true
padding_left = 1
padding_right = 1
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Theme.ColorScheme = "custom"

	visuals, err := VisualsFromConfig(cfg, theme.TrueColor, configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := visuals.Theme.BG("panel"), "\x1b[48;2;171;205;239m"; got != want {
		t.Fatalf("panel bg = %q, want %q", got, want)
	}
	if got := visuals.Styles["time"]; got.FG != "base.bg" || got.BG != "panel" || !got.Bold {
		t.Fatalf("file time style = %+v", got)
	}
}

func TestVisualsFromConfigRejectsMissingThemeFile(t *testing.T) {
	cfg := config.Default()
	cfg.Theme.ColorScheme = "missing"

	_, err := VisualsFromConfig(cfg, theme.TrueColor, filepath.Join(t.TempDir(), "config.toml"))
	if err == nil || !strings.Contains(err.Error(), "theme \"missing\" not found") {
		t.Fatalf("VisualsFromConfig error = %v, want missing theme", err)
	}
}

func TestVisualsFromConfigRejectsInvalidStyleColor(t *testing.T) {
	cfg := config.Default()
	cfg.Styles = map[string]config.StyleConfig{
		"time": {FG: "not-a-color"},
	}

	_, err := VisualsFromConfig(cfg, theme.TrueColor, "")
	if err == nil || !strings.Contains(err.Error(), "style.time.fg") {
		t.Fatalf("VisualsFromConfig error = %v, want style.time.fg", err)
	}
}
