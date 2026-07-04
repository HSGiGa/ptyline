package app

import (
	"sync/atomic"

	"github.com/hsgiga/ptyline/internal/app/bar"
	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/diagnostics"
	"github.com/hsgiga/ptyline/internal/proxy"
	"github.com/hsgiga/ptyline/internal/pty"
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/runtimeenv"
	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/layout"
	"github.com/hsgiga/ptyline/internal/status/renderer"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// appState groups the mutable application-level state that changes on reload and
// resize. All mutable fields are POINTERS to the corresponding local variables in
// run(), so existing closures continue to work without change while reloadConfig
// and resize can be promoted to methods (ARCHITECTURE.md §A1).
//
// Promoting closures to methods enables them to be unit-tested independently of
// the PTY: mock writer/filter/ctrl/sup via interfaces in a follow-up step.
type appState struct {
	// Mutable state — pointers so the method and existing closures share the same
	// underlying variable. reloadConfig writes through these pointers.
	resolvedCfg        *config.Config
	area               *reserved.Area
	barRows            *[]bar.RowSpec
	visuals            *bar.Visuals
	render             **renderer.Renderer
	animState          **renderer.AnimationState
	projectOverlayPath *atomic.Value
	projectConfigCache *map[string]string

	// Immutable config inputs — read by reloadConfig.
	cfg                 config.Config
	cfgExplicitDisabled map[string]bool // module ids cfg explicitly set enabled=false on
	cliOverlay          string          // resolved overlay path (from --overlay flag)
	shell               string          // child shell label, drives color_scheme = "default" resolution

	// Observability — created once, pointer gives full lifecycle access.
	diagState *diagnostics.State

	// I/O dependencies — set once at startup, read-only after that.
	writer  *proxy.TerminalWriter
	filter  *proxy.AnsiFilter
	ctrl    *terminal.Controller
	sup     *pty.Supervisor
	profile runtimeenv.Profile
	opts    options

	// termState is a pointer to the status.StatusState in run(), shared so
	// reloadConfig can read the current terminal dimensions.
	termState *status.StatusState

	// Callbacks wired in run() after construction.
	newEngine         func(cols int) *layout.Engine
	configureRenderer func(*renderer.Renderer)
	updateModules     func()
}
