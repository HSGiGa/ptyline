package app

import (
	"os"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/runtimeenv"
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

var builtinModuleIDs = map[string]bool{
	"time": true, "hostname": true, "user": true, "runtime": true, "shell": true,
	"env": true, "cwd": true, "ssh": true, "git": true, "command": true,
}

func customModuleSource(id string, cfg config.ModuleConfig) string {
	if cfg.Source != "" {
		return cfg.Source
	}
	if cfg.Provider == "command" {
		return "exec"
	}
	if cfg.Provider != "" {
		return cfg.Provider
	}
	if !builtinModuleIDs[id] {
		return "exec"
	}
	return ""
}
