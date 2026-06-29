// Package app wires the components together and owns the run sequence. It parses
// the CLI, loads config, detects the runtime, then constructs the terminal
// controller, PTY supervisor, ANSI filter, and event loop. It is the only place
// that knows how the pieces fit; the packages themselves stay decoupled.
package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
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

// ptyDrainGrace bounds how long the ChildExited emitter waits for the PTY reader
// to drain the child's final output after the shell is reaped. A lingering
// grandchild can keep the slave open indefinitely, so this caps the wait.
const ptyDrainGrace = 100 * time.Millisecond

// userModEntry tracks a running user-defined module goroutine (exec or custom-time).
type userModEntry struct {
	cancel   context.CancelFunc
	command  string             // non-empty for exec; used to detect restarts
	interval time.Duration      // used to detect restarts
	exec     *execModuleRuntime // nil for custom-time modules
}

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
	case opts.Reload:
		return sendReloadSignal()
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
	sup.SetEnv("PTYLINE_PID", strconv.Itoa(os.Getpid()))
	if err := sup.Start(pty.Size{Cols: size.Cols, Rows: size.Rows}); err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: pty:", err)
		return 1
	}

	// --- Event loop + ANSI/OSC filter. ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := event.NewBus(256)
	filter := proxy.NewAnsiFilter(area)
	filter.SetRows(size.Rows)
	loop := proxy.NewLoop(bus, filter)
	writer := proxy.NewTerminalWriter(os.Stdout)
	top, count := bar.Geometry(area, size.Rows, len(barRows))
	writer.SetBarRows(top, count)
	state := status.NewState()
	state.Resize(size.Cols, size.Rows, false)
	// cwdHolder is read by git refresh goroutines and written by the loop goroutine.
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

	cmdTracker := command.NewTracker(resolvedCfg.Modules["command"])

	// sshBaseSnap is the env-based SSH snapshot (inbound SSH detection); stable
	// for the session lifetime and NOT reset on config reload.
	sshBaseSnap := modules.NewSSH().Refresh(context.Background())
	state.UpdateModule(sshBaseSnap)
	sshAnim := modules.NewSSHAnimator(sshBaseSnap)

	visuals, err := bar.VisualsFromConfig(resolvedCfg, colorMode(profile.Capabilities.Color), opts.ConfigPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: theme:", err)
		return 1
	}
	var shellStyles map[string]style.Style
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
	newEngine := func(cols int) *layout.Engine {
		return layout.NewWithMinBlock(cols, resolvedCfg.Bar.MinBlockWidth)
	}
	var changeAnimating atomic.Bool
	configureRenderer := func(r *renderer.Renderer) {
		r.SetJustify(renderer.Justify(resolvedCfg.Bar.Justify))
		r.SetStyles(mergedStyles())
		r.SetAnimations(bar.AnimationsFromConfig(resolvedCfg.Modules))
		r.SetTemplates(bar.TemplateSpecs(resolvedCfg))
		r.SetIcons(bar.IconSpecs(resolvedCfg))
		r.SetChangeFlag(&changeAnimating)
	}
	render := renderer.New(newEngine(int(size.Cols)), visuals.Theme)
	configureRenderer(render)
	var resizePending bool
	renderFn := func() []string { return bar.Render(render, state, barRows) }
	redraw := func() {
		if resizePending {
			return
		}
		writer.RequestRedraw()
		_ = writer.FlushBarFrameLazy(renderFn)
	}
	altCoord := &altScreenCoordinator{
		ctrl: ctrl, sup: sup, writer: writer, state: &state, area: &area, redraw: redraw,
	}

	// projectOverlayPath tracks the active project .ptyline so we skip rebuilds
	// when cwd changes but the nearest overlay is the same file.
	var projectOverlayPath atomic.Value
	projectOverlayPath.Store(initProjectOverlay)

	// --- Module lifecycle ---
	//
	// Built-in interval modules (time, date, git) have goroutines bound to ctx
	// and live for the full app lifetime. On reload only their config is updated
	// in-place (Format) or the goroutine is restarted if the interval changed.
	//
	// User-defined modules (exec, custom-time) each get their own cancel context
	// stored in userMods. On reload we diff old vs new by ID: cancel removed ones,
	// restart changed ones (command or interval), leave unchanged ones alone.
	//
	// The animation ticker is always restarted on reload because it needs the
	// updated module animation config.

	scheduler := status.NewScheduler(func(snap status.ModuleSnapshot) {
		bus.SendCtx(ctx, event.ModuleUpdated{ID: string(snap.ID), Snapshot: snap})
	})

	var (
		timeModule   *modules.Time
		dateModule   *modules.Time
		gitMod       *modules.Git
		gitCancel    context.CancelFunc
		timeCancel   context.CancelFunc
		dateCancel   context.CancelFunc
		tickerCancel context.CancelFunc
	)
	userMods := map[string]*userModEntry{}
	var execModules []*execModuleRuntime
	var gitRefreshing atomic.Bool

	// refreshGitMod runs RefreshAll on the given git module off the event loop and
	// emits every sub-module snapshot. gm is passed explicitly so callers off the
	// event loop (the ticker goroutine) never read the shared gitMod variable,
	// which the loop rewrites on reload.
	refreshGitMod := func(gm *modules.Git, expectedCWD string) {
		if !gitRefreshing.CompareAndSwap(false, true) {
			return
		}
		go func() {
			defer gitRefreshing.Store(false)
			rctx, rcancel := context.WithTimeout(ctx, time.Second)
			defer rcancel()
			snaps := gm.RefreshAll(rctx)
			if expectedCWD != "" {
				if cur, _ := cwdHolder.Load().(string); cur != expectedCWD {
					return
				}
			}
			for _, snap := range snaps {
				bus.SendCtx(ctx, event.ModuleUpdated{ID: string(snap.ID), Snapshot: snap})
			}
		}()
	}

	// refreshGit triggers a git refresh against the current gitMod. It reads the
	// shared gitMod variable, so it must be called from the event loop goroutine
	// only (cwd/command events). The ticker uses refreshGitMod with a captured
	// module instead.
	refreshGit := func(expectedCWD string) {
		refreshGitMod(gitMod, expectedCWD)
	}

	// startGitTicker launches a goroutine that refreshes gm on its own interval.
	// gm is captured by value so the goroutine never touches the shared gitMod
	// variable. Unlike the generic scheduler, it calls RefreshAll so all git
	// sub-module snapshots are emitted on every tick.
	startGitTicker := func(gCtx context.Context, gm *modules.Git) {
		go func() {
			ticker := time.NewTicker(gm.Interval())
			defer ticker.Stop()
			for {
				select {
				case <-gCtx.Done():
					return
				case <-ticker.C:
					refreshGitMod(gm, "")
				}
			}
		}()
	}

	restartTicker := func() {
		if tickerCancel != nil {
			tickerCancel()
		}
		tCtx, tCancel := context.WithCancel(ctx)
		tickerCancel = tCancel
		bar.StartTicker(tCtx, bus, resolvedCfg.Modules, cmdTracker.Animating(), &changeAnimating)
	}

	rebuildExecModules := func() {
		var em []*execModuleRuntime
		for _, entry := range userMods {
			if entry.exec != nil {
				em = append(em, entry.exec)
			}
		}
		execModules = em
	}

	startUserMod := func(id string, mcfg config.ModuleConfig) {
		src := config.ModuleSource(id, mcfg)
		interval := moduleInterval(mcfg, time.Second)
		mCtx, mCancel := context.WithCancel(ctx)
		switch src {
		case "exec":
			if mcfg.Command == "" {
				mCancel()
				return
			}
			em := newExecModuleRuntime(id, mcfg)
			em.start(mCtx, bus)
			state.UpdateModule(em.module.Refresh(context.Background()))
			userMods[id] = &userModEntry{cancel: mCancel, command: mcfg.Command, interval: interval, exec: em}
		case "time":
			tm := modules.NewTimeWithID(id, mcfg.Format, interval)
			scheduler.Start(mCtx, tm, 2*time.Second)
			state.UpdateModule(tm.Refresh(context.Background()))
			userMods[id] = &userModEntry{cancel: mCancel, interval: interval}
		default:
			mCancel()
		}
	}

	// initModules creates the built-in modules and starts all goroutines. Called
	// exactly once at startup.
	initModules := func() {
		timeModule = modules.NewTime(resolvedCfg.Modules["time"].Format, moduleInterval(resolvedCfg.Modules["time"], time.Second))
		dateModule = modules.NewDate(resolvedCfg.Modules["date"].Format, moduleInterval(resolvedCfg.Modules["date"], time.Minute))
		gcfg := resolvedCfg.Modules["git"]
		gitMod = modules.NewGit(moduleInterval(gcfg, 2*time.Second), time.Second,
			func() string { s, _ := cwdHolder.Load().(string); return s }).
			WithFormat(gcfg.Format, gcfg.Separator, gcfg.MaxWidth)

		builtins := []status.Module{
			timeModule, dateModule,
			modules.NewHostname(), modules.NewUser(), modules.NewRuntime(profile),
			modules.NewShell(argv), modules.NewEnv(resolvedCfg.Modules["env"].Env),
		}
		for _, m := range builtins {
			state.UpdateModule(m.Refresh(context.Background()))
		}

		gitCtx, gCancel := context.WithCancel(ctx)
		gitCancel = gCancel
		tCtx, tCancel := context.WithCancel(ctx)
		timeCancel = tCancel
		scheduler.Start(tCtx, timeModule, 2*time.Second)
		dCtx, dCancel := context.WithCancel(ctx)
		dateCancel = dCancel
		scheduler.Start(dCtx, dateModule, 30*time.Second)
		startGitTicker(gitCtx, gitMod)

		for id, mcfg := range resolvedCfg.Modules {
			if config.ModuleSource(id, mcfg) == "" {
				continue // skip built-in IDs that have no user-defined source
			}
			startUserMod(id, mcfg)
		}
		rebuildExecModules()
		restartTicker()
	}

	// updateModules applies a new resolvedCfg to the running module set:
	// built-ins are updated in-place, user-defined modules are diff'ed by ID.
	updateModules := func() {
		// Built-in time/date: restart goroutine when format or interval changes to
		// avoid a data race between the scheduler goroutine (reader) and this write.
		newTimeFmt := resolvedCfg.Modules["time"].Format
		newTimeInterval := moduleInterval(resolvedCfg.Modules["time"], time.Second)
		if (newTimeFmt != "" && newTimeFmt != timeModule.Format) || newTimeInterval != timeModule.Interval() {
			if newTimeFmt == "" {
				newTimeFmt = timeModule.Format
			}
			timeCancel()
			timeModule = modules.NewTime(newTimeFmt, newTimeInterval)
			tCtx, tCancel := context.WithCancel(ctx)
			timeCancel = tCancel
			scheduler.Start(tCtx, timeModule, 2*time.Second)
			state.UpdateModule(timeModule.Refresh(context.Background()))
		}
		newDateFmt := resolvedCfg.Modules["date"].Format
		newDateInterval := moduleInterval(resolvedCfg.Modules["date"], time.Minute)
		if (newDateFmt != "" && newDateFmt != dateModule.Format) || newDateInterval != dateModule.Interval() {
			if newDateFmt == "" {
				newDateFmt = dateModule.Format
			}
			dateCancel()
			dateModule = modules.NewDate(newDateFmt, newDateInterval)
			dCtx, dCancel := context.WithCancel(ctx)
			dateCancel = dCancel
			scheduler.Start(dCtx, dateModule, 30*time.Second)
			state.UpdateModule(dateModule.Refresh(context.Background()))
		}

		// Git: restart when the polling interval or the composite format changed.
		gcfg := resolvedCfg.Modules["git"]
		newGitInterval := moduleInterval(gcfg, 2*time.Second)
		if !gitMod.SameConfig(newGitInterval, gcfg.Format, gcfg.Separator, gcfg.MaxWidth) {
			gitCancel()
			gitMod = modules.NewGit(newGitInterval, time.Second,
				func() string { s, _ := cwdHolder.Load().(string); return s }).
				WithFormat(gcfg.Format, gcfg.Separator, gcfg.MaxWidth)
			gitCtx, gCancel := context.WithCancel(ctx)
			gitCancel = gCancel
			startGitTicker(gitCtx, gitMod)
			for _, snap := range gitMod.RefreshAll(context.Background()) {
				state.UpdateModule(snap)
			}
		}

		// User-defined: build the desired set from new config.
		newUserCfg := map[string]config.ModuleConfig{}
		for id, mcfg := range resolvedCfg.Modules {
			src := config.ModuleSource(id, mcfg)
			if (src == "exec" && mcfg.Command != "") || src == "time" {
				newUserCfg[id] = mcfg
			}
		}

		// Cancel goroutines for removed modules.
		for id, entry := range userMods {
			if _, stillExists := newUserCfg[id]; !stillExists {
				entry.cancel()
				delete(userMods, id)
			}
		}

		// Update existing modules or start new ones.
		for id, mcfg := range newUserCfg {
			newInterval := moduleInterval(mcfg, time.Second)
			if existing, exists := userMods[id]; exists {
				needsRestart := newInterval != existing.interval
				if config.ModuleSource(id, mcfg) == "exec" {
					needsRestart = needsRestart || mcfg.Command != existing.command
				}
				if !needsRestart {
					// Goroutine keeps running; format changes are picked up via
					// configureRenderer (templates/animations) on the next redraw.
					continue
				}
				existing.cancel()
				delete(userMods, id)
			}
			startUserMod(id, mcfg)
		}
		rebuildExecModules()

		// Ticker needs the updated animation config.
		restartTicker()

		// Re-snapshot static (non-goroutine) built-ins in case config changed.
		for _, m := range []status.Module{
			modules.NewHostname(), modules.NewUser(),
			modules.NewRuntime(profile), modules.NewShell(argv),
		} {
			state.UpdateModule(m.Refresh(context.Background()))
		}
	}

	// reloadConfig rebuilds the resolved config and bar. force=true skips the
	// project-path equality guard (used on explicit --reload). Returns true when
	// the config was successfully applied. Called from the event loop goroutine,
	// so no locking is needed for closure-captured variables.
	reloadConfig := func(newProjectPath string, force bool) bool {
		old, _ := projectOverlayPath.Load().(string)
		if !force && old == newProjectPath {
			return false
		}
		projectOverlayPath.Store(newProjectPath)
		newCfg, err := config.ApplyOverlays(cfg, cliOverlay, newProjectPath)
		if err != nil {
			return false // keep running with old config on parse error
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
		render = renderer.New(newEngine(int(state.Terminal.Cols)), visuals.Theme)
		configureRenderer(render)
		updateModules()
		return true
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
			altCoord.FlushPending()
			return nil
		},
		ResizeRequest: func(cols, rows uint16) {
			if !resizePending {
				_, _ = ctrl.Write([]byte(terminal.HideCursor))
			}
			resizePending = true
			resizeDebouncer.Send(terminal.Size{Cols: cols, Rows: rows})
		},
		ResizeCommit: func(cols, rows uint16) {
			alt := filter.AltActive()
			resizePending = false
			if !alt && rows > state.Terminal.Rows {
				_ = writer.ClearBar()
			}
			state.Resize(cols, rows, alt)
			top, count := bar.Geometry(area, rows, len(barRows))
			writer.SetBarRows(top, count)
			render = renderer.New(newEngine(int(cols)), visuals.Theme)
			configureRenderer(render)
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
					reloadConfig(newPath, false)
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
		ConfigReload: func() {
			newBase, err := config.Load(opts.ConfigPath)
			if err != nil {
				return // keep running on bad config
			}
			prevCfg := cfg
			cfg = newBase
			currentPath, _ := projectOverlayPath.Load().(string)
			if !reloadConfig(currentPath, true) {
				cfg = prevCfg // ApplyOverlays failed: restore base to stay consistent
			}
		},
	})
	filter.SetAltHandler(altCoord.SetPending)
	proxy.StartReader(ctx, bus, os.Stdin, func(data []byte) event.AppEvent { return event.StdinInput{Data: data} })
	ptyDrained := proxy.StartReader(ctx, bus, sup.PTY(), func(data []byte) event.AppEvent { return event.PtyOutput{Data: data} })
	go func() {
		code, _ := sup.Wait()
		// Let the PTY reader drain and enqueue the child's final output before
		// ChildExited (which ends the loop and would otherwise drop it). Bounded by
		// a grace window so a lingering grandchild holding the slave open (e.g.
		// `sh -c 'sleep 100 &'`) cannot hang the wrapper after the shell exits.
		select {
		case <-ptyDrained:
		case <-time.After(ptyDrainGrace):
		}
		bus.SendCtx(ctx, event.ChildExited{Code: code})
	}()
	proxy.StartSignals(ctx, bus)
	initModules()
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
