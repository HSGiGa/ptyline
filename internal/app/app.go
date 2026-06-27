// Package app wires the components together and owns the run sequence. It parses
// the CLI, loads config, detects the runtime, then constructs the terminal
// controller, PTY supervisor, ANSI filter, and event loop. It is the only place
// that knows how the pieces fit; the packages themselves stay decoupled.
package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hsgiga/ptyline/internal/app/bar"
	"github.com/hsgiga/ptyline/internal/command"
	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/modules"
	"github.com/hsgiga/ptyline/internal/proxy"
	"github.com/hsgiga/ptyline/internal/pty"
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/runtimeenv"
	"github.com/hsgiga/ptyline/internal/shellintegration"
	"github.com/hsgiga/ptyline/internal/shellintegration/shellcolors"
	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/layout"
	"github.com/hsgiga/ptyline/internal/status/renderer"
	"github.com/hsgiga/ptyline/internal/status/style"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// Run is the program entrypoint. It returns the process exit code, which for the
// run path equals the child's exit code (spec §8.2). See spec §14 for the CLI.
func Run(args []string, version string) int {
	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ptyline:", err)
		return 2
	}

	switch {
	case opts.ShowVersion:
		fmt.Println("ptyline", version)
		return 0
	case opts.ShowHelp:
		fmt.Print(usage)
		return 0
	case opts.InitShell != "":
		script, ok := shellintegration.Script(opts.InitShell)
		if !ok {
			fmt.Fprintf(os.Stderr, "ptyline: no integration for shell %q (supported: %v)\n",
				opts.InitShell, shellintegration.Supported())
			return 1
		}
		fmt.Print(script)
		return 0
	}

	return run(opts)
}

// run constructs and drives the wrapper pipeline.
func run(opts options) int {
	profile := runtimeenv.Detect()
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: config:", err)
		return 1
	}

	cliOverlay := config.ResolveOverlayPath(opts.OverlayPath)
	var initProjectOverlay string
	if !opts.NoProjectPtyline {
		if cwd, err := os.Getwd(); err == nil {
			initProjectOverlay, _ = config.FindProjectConfig(cwd)
		}
	}
	resolvedCfg, err := config.ApplyOverlays(cfg, cliOverlay, initProjectOverlay)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: overlay:", err)
		return 1
	}

	barRows := bar.BuildRows(resolvedCfg)
	area := reserved.Area{Edge: reserved.Bottom, Rows: uint16(len(barRows))}
	argv := resolveChild(opts.Child, resolvedCfg, profile)

	// --- Terminal: enter raw mode; ALWAYS restore on the way out (spec §15). ---
	ctrl := terminal.New(os.Stdin, os.Stdout)
	if err := ctrl.Enter(); err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: terminal:", err)
		return 1
	}
	defer func() { _ = ctrl.Restore() }()

	size, err := terminal.QuerySize()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: warning: cannot query terminal size, using 80x24")
	}
	ctrl.ApplyScrollRegion(size, area)
	_, _ = ctrl.Write([]byte(terminal.ClearScreen + terminal.CursorTo(1, 1)))

	// --- Child PTY sized to rows-minus-reserved (spec §8.2). ---
	sup := pty.New(argv, area)
	sup.SetEnv("PTYLINE_ENV_NAMES", strings.Join(resolvedCfg.Modules["env"].Env, ","))
	if err := sup.Start(pty.Size{Cols: size.Cols, Rows: size.Rows}); err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: pty:", err)
		return 1
	}

	// --- Event loop + ANSI/OSC filter. ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // stops module refresh tickers on every exit path
	bus := event.NewBus(256)
	filter := proxy.NewAnsiFilter(area)
	filter.SetRows(size.Rows)
	loop := proxy.NewLoop(bus, filter)
	writer := proxy.NewTerminalWriter(os.Stdout)
	top, count := bar.Geometry(area, size.Rows, len(barRows))
	writer.SetBarRows(top, count)
	state := status.NewState()
	state.Resize(size.Cols, size.Rows, false)
	// cwdHolder is read by the git module's refresh goroutine and written by the
	// loop goroutine on cwd shell-meta, so it must be race-free.
	var cwdHolder atomic.Value
	cwdHolder.Store("")
	if cwd, err := os.Getwd(); err == nil {
		state.Shell.CWD = cwd
		cwdHolder.Store(cwd)
		state.UpdateModule(status.ModuleSnapshot{
			ID:        "cwd",
			Value:     status.Text(modules.AbbreviateHome(cwd, "")),
			UpdatedAt: time.Now(),
		})
	}
	timeModule := modules.NewTime(resolvedCfg.Modules["time"].Format, moduleInterval(resolvedCfg.Modules["time"], time.Second))
	cmdTracker := command.NewTracker(resolvedCfg.Modules["command"])
	gitModule := modules.NewGit(moduleInterval(resolvedCfg.Modules["git"], 2*time.Second), time.Second, func() string {
		s, _ := cwdHolder.Load().(string)
		return s
	})
	envModule := modules.NewEnv(resolvedCfg.Modules["env"].Env)
	// Build user-defined modules. For known built-in IDs, an empty source keeps
	// the built-in behavior; for unknown IDs, empty source defaults to exec.
	var execModules []*execModuleRuntime
	var customTimeModules []status.Module
	for id, mcfg := range resolvedCfg.Modules {
		switch config.ModuleSource(id, mcfg) {
		case "exec":
			if mcfg.Command == "" {
				continue
			}
			execModules = append(execModules, newExecModuleRuntime(id, mcfg))
		case "time":
			customTimeModules = append(customTimeModules, modules.NewTimeWithID(
				id,
				mcfg.Format,
				moduleInterval(mcfg, time.Second),
			))
		case "template":
			// resolved in renderer from cached snapshots; no runtime needed
		}
	}
	// Initial synchronous paint so the bar shows values immediately; the
	// scheduler then refreshes interval-driven modules (e.g. time) in the
	// background and feeds snapshots back through ModuleUpdated events.
	// sshBaseSnap is the env-based SSH snapshot (inbound SSH detection); it is
	// reused as the fallback when an outbound ssh_end event arrives.
	sshBaseSnap := modules.NewSSH().Refresh(nil)
	builtins := []status.Module{
		timeModule,
		modules.NewHostname(),
		modules.NewUser(),
		modules.NewRuntime(profile),
		modules.NewShell(argv),
		envModule,
	}
	for _, m := range execModules {
		builtins = append(builtins, m.module)
	}
	builtins = append(builtins, customTimeModules...)
	for _, module := range builtins {
		state.UpdateModule(module.Refresh(nil))
	}
	state.UpdateModule(sshBaseSnap)
	sshAnim := modules.NewSSHAnimator(sshBaseSnap)
	scheduler := status.NewScheduler(func(snap status.ModuleSnapshot) {
		bus.SendCtx(ctx, event.ModuleUpdated{ID: string(snap.ID), Snapshot: snap})
	})
	var gitRefreshing atomic.Bool
	refreshGit := func(expectedCWD string) {
		if !gitRefreshing.CompareAndSwap(false, true) {
			return // a refresh is already in flight; the scheduler will pick up the next tick
		}
		go func() {
			defer gitRefreshing.Store(false)
			rctx, rcancel := context.WithTimeout(ctx, time.Second)
			defer rcancel()
			snap := gitModule.Refresh(rctx)
			if expectedCWD != "" {
				current, _ := cwdHolder.Load().(string)
				if current != expectedCWD {
					return
				}
			}
			bus.SendCtx(ctx, event.ModuleUpdated{ID: "git", Snapshot: snap})
		}()
	}
	visuals, err := bar.VisualsFromConfig(resolvedCfg, colorMode(profile.Capabilities.Color), opts.ConfigPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: theme:", err)
		return 1
	}
	// shellStyles holds module-style overrides received from the shell via the
	// OSC 777 "colors" key. They are the lowest-priority override: config and
	// theme file styles always win (see mergedStyles).
	var shellStyles map[string]style.Style
	// mergedStyles merges shellStyles (shell auto-detected) with visuals.Styles
	// (theme file + config inline). visuals.Styles always takes priority so that
	// an explicit user theme is never overridden by shell color sync.
	mergedStyles := func() map[string]style.Style {
		if len(shellStyles) == 0 {
			return visuals.Styles
		}
		merged := make(map[string]style.Style, len(shellStyles)+len(visuals.Styles))
		for k, v := range shellStyles {
			merged[k] = v
		}
		for k, v := range visuals.Styles {
			merged[k] = v // config/theme always wins
		}
		return merged
	}
	render := renderer.New(layout.New(int(size.Cols)), visuals.Theme)
	render.SetStyles(mergedStyles())
	render.SetAnimations(bar.AnimationsFromConfig(resolvedCfg.Modules))
	render.SetTemplates(bar.TemplateSpecs(resolvedCfg))
	render.SetIcons(bar.IconSpecs(resolvedCfg))
	var resizePending bool
	renderFn := func() []string { return bar.Render(render, state, barRows) }
	redraw := func() {
		if resizePending {
			return
		}
		writer.RequestRedraw()
		_ = writer.FlushBarFrameLazy(renderFn)
	}
	// altCoord sequences the alt-screen entry/exit protocol (spec §11):
	// the filter records the transition; WriteOutput flushes it after bytes land.
	altCoord := &altScreenCoordinator{
		ctrl: ctrl, sup: sup, writer: writer, state: &state, area: &area, redraw: redraw,
	}
	// projectOverlayPath tracks the currently active project .ptyline path so we
	// can skip the rebuild when cwd changes but the nearest .ptyline stays the same.
	var projectOverlayPath atomic.Value
	projectOverlayPath.Store(initProjectOverlay)
	// reloadOverlay rebuilds the resolved config and bar when the project overlay
	// path changes. Called from the ShellMeta "cwd" handler (event loop goroutine),
	// so no locking is needed for the closure-captured variables.
	reloadOverlay := func(newProjectPath string) {
		old, _ := projectOverlayPath.Load().(string)
		if old == newProjectPath {
			return
		}
		projectOverlayPath.Store(newProjectPath)
		newCfg, err := config.ApplyOverlays(cfg, cliOverlay, newProjectPath)
		if err != nil {
			return // keep running with old config on parse error
		}
		resolvedCfg = newCfg
		newBarRows := bar.BuildRows(resolvedCfg)
		newArea := reserved.Area{Edge: reserved.Bottom, Rows: uint16(len(newBarRows))}
		if newArea.Rows != area.Rows {
			curSize := terminal.Size{Cols: state.Terminal.Cols, Rows: state.Terminal.Rows}
			_ = writer.ClearBar()
			area = newArea
			filter.SetArea(area)
			sup.SetArea(area)
			top, count := bar.Geometry(area, curSize.Rows, len(newBarRows))
			writer.SetBarRows(top, count)
			_ = sup.Resize(pty.Size{Cols: curSize.Cols, Rows: curSize.Rows})
			ctrl.ApplyScrollRegion(curSize, area)
		}
		barRows = newBarRows
		if newVisuals, err := bar.VisualsFromConfig(resolvedCfg, colorMode(profile.Capabilities.Color), opts.ConfigPath); err == nil {
			visuals = newVisuals
		}
		render = renderer.New(layout.New(int(state.Terminal.Cols)), visuals.Theme)
		render.SetStyles(mergedStyles())
		render.SetAnimations(bar.AnimationsFromConfig(resolvedCfg.Modules))
		render.SetTemplates(bar.TemplateSpecs(resolvedCfg))
		render.SetIcons(bar.IconSpecs(resolvedCfg))
		redraw()
	}
	resizeDebouncer := proxy.NewResizeDebouncer(proxy.ResizeCommitDelay)
	resizeDebouncer.Start(ctx, bus)
	var pendingRefreshCommand string
	loop.SetHandlers(proxy.Handlers{
		WriteInput: func(data []byte) error {
			if len(data) > 0 {
				cmdTracker.RecordKeystroke()
			}
			_, err := sup.PTY().Write(data)
			return err
		},
		WriteOutput: func(data []byte) error {
			if err := writer.WriteChild(data); err != nil {
				return err
			}
			if len(data) > 0 {
				if changed := cmdTracker.RecordOutput(&state.Shell); changed {
					state.ActiveCommandAnimating = true
				}
			}
			writer.InvalidateBar()
			// The child bytes (including any ?1049h/l) have now reached the
			// terminal; flush a pending alt-screen transition on the correct screen.
			altCoord.FlushPending()
			return nil
		},
		ResizeRequest: func(cols, rows uint16) {
			if !resizePending {
				// Hide the cursor for the duration of the resize burst so it does
				// not visibly teleport while the terminal reflows and the child
				// repaints (tmux does the same around its redraws).
				_, _ = ctrl.Write([]byte(terminal.HideCursor))
			}
			resizePending = true
			resizeDebouncer.Send(terminal.Size{Cols: cols, Rows: rows})
		},
		ResizeCommit: func(cols, rows uint16) {
			alt := filter.AltActive()
			resizePending = false
			// Clearing only on a committed grow avoids ghost bars during resize.
			if !alt && rows > state.Terminal.Rows {
				_ = writer.ClearBar()
			}
			state.Resize(cols, rows, alt)
			top, count := bar.Geometry(area, rows, len(barRows))
			writer.SetBarRows(top, count)
			render = renderer.New(layout.New(int(cols)), visuals.Theme)
			render.SetStyles(mergedStyles())
			render.SetAnimations(bar.AnimationsFromConfig(resolvedCfg.Modules))
			render.SetTemplates(bar.TemplateSpecs(resolvedCfg))
			render.SetIcons(bar.IconSpecs(resolvedCfg))
			if alt {
				_ = sup.ResizeFull(pty.Size{Cols: cols, Rows: rows})
				ctrl.ResetScrollRegion()
				_, _ = ctrl.Write([]byte(terminal.ShowCursor))
				return
			}
			_ = sup.Resize(pty.Size{Cols: cols, Rows: rows})
			ctrl.ApplyScrollRegion(terminal.Size{Cols: cols, Rows: rows}, area)
			_, _ = ctrl.Write([]byte(terminal.ShowCursor))
		},
		ShellMeta: func(key, value string) {
			state.ApplyShellMeta(key, value)
			if key == shellintegration.KeyCommand && value != "" {
				pendingRefreshCommand = state.Shell.LastCommand
			}
			if key == shellintegration.KeyColors {
				shellStyles = shellcolors.ParseToStyles(value)
				render.SetStyles(mergedStyles())
				redraw()
				return
			}
			if key == shellintegration.KeySSHStart {
				state.UpdateModule(sshAnim.OnSSHStart(state.Shell.SSHTarget))
			}
			if key == shellintegration.KeySSHEnd {
				state.UpdateModule(sshAnim.OnSSHEnd())
			}
			if key == "cwd" {
				cwdHolder.Store(state.Shell.CWD)
				state.UpdateModule(status.ModuleSnapshot{
					ID:        "cwd",
					Value:     status.Text(modules.AbbreviateHome(state.Shell.CWD, "")),
					UpdatedAt: time.Now(),
				})
				refreshGit(state.Shell.CWD)
				if !opts.NoProjectPtyline {
					newPath, _ := config.FindProjectConfig(state.Shell.CWD)
					reloadOverlay(newPath)
				}
			}
			if key == shellintegration.KeyEnv {
				state.UpdateModule(status.ModuleSnapshot{
					ID:        "env",
					Value:     status.Text(value),
					UpdatedAt: time.Now(),
				})
			}
			if key == shellintegration.KeyExitCode {
				if shouldRefreshAfterExit(value, pendingRefreshCommand, state.Shell.LastCommand) {
					for _, m := range execModules {
						m.refreshAfterCommand(ctx, bus, pendingRefreshCommand)
					}
				}
				pendingRefreshCommand = ""
			}
			if snap := cmdTracker.ApplyShellMeta(key, &state); snap != nil {
				state.UpdateModule(*snap)
			}
		},
		ModuleUpdated: func(_ string, snapshot any) {
			if snap, ok := snapshot.(status.ModuleSnapshot); ok {
				state.UpdateModule(snap)
			}
		},
		Tick: func() {
			state.AnimationPhase++
			if snap := sshAnim.Tick(state.Shell.SSHTarget); snap != nil {
				state.UpdateModule(*snap)
			}
			if snap := cmdTracker.Tick(&state); snap != nil {
				state.UpdateModule(*snap)
			}
		},
		Redraw:    redraw,
		Terminate: func(sig string) { _ = sup.TerminateGroup(sig) },
	})
	filter.SetAltHandler(altCoord.SetPending)
	proxy.StartReader(ctx, bus, os.Stdin, func(data []byte) event.AppEvent { return event.StdinInput{Data: data} })
	proxy.StartReader(ctx, bus, sup.PTY(), func(data []byte) event.AppEvent { return event.PtyOutput{Data: data} })
	go func() { code, _ := sup.Wait(); bus.SendCtx(ctx, event.ChildExited{Code: code}) }()
	proxy.StartSignals(ctx, bus)
	scheduler.Start(ctx, timeModule, 2*time.Second)
	scheduler.Start(ctx, gitModule, time.Second)
	for _, m := range customTimeModules {
		scheduler.Start(ctx, m, 2*time.Second)
	}
	for _, m := range execModules {
		m.start(ctx, bus)
	}
	bar.StartTicker(ctx, bus, resolvedCfg.Modules, cmdTracker.Animating())
	// Paint git as soon as possible without blocking startup if git hangs.
	refreshGit("")
	redraw()
	code, err := loop.Run()
	_ = writer.ClearBar()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ptyline:", err)
		return 1
	}
	return code
}
