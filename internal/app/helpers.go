package app

import (
	"os"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/runtimeenv"
	"github.com/hsgiga/ptyline/internal/status/icons"
	"github.com/hsgiga/ptyline/internal/status/theme"
)

// colorMode maps the detected terminal color level to a theme render mode.
func colorMode(level runtimeenv.ColorLevel) theme.Mode {
	switch level {
	case runtimeenv.ColorTrue:
		return theme.TrueColor
	case runtimeenv.Color256:
		return theme.Color256
	case runtimeenv.ColorBasic:
		return theme.Color16
	default:
		return theme.NoColor
	}
}

// resolveChild picks the command to run inside the PTY: explicit argv, else the
// configured shell, else $SHELL (spec §14).
func resolveChild(child []string, cfg config.Config, _ runtimeenv.Profile) []string {
	if len(child) > 0 {
		return child
	}
	if cfg.Shell != "" && cfg.Shell != "auto" {
		return []string{cfg.Shell}
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return []string{sh}
	}
	return []string{"/bin/sh"}
}

// moduleInterval returns the configured refresh interval or fallback.
func moduleInterval(cfg config.ModuleConfig, fallback time.Duration) time.Duration {
	if cfg.IntervalMS <= 0 {
		return fallback
	}
	return time.Duration(cfg.IntervalMS) * time.Millisecond
}

// gitBranchIcon uses the Nerd Font branch glyph only when the user selected a
// Nerd Font preset. The fallback is a normal-font branch symbol.
func gitBranchIcon(preset string) string {
	if icons.Preset(preset) == icons.PresetNerdFont {
		return ""
	}
	return "⎇"
}
