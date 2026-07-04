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
	"sync"
	"sync/atomic"
	"time"

	"github.com/hsgiga/ptyline/internal/app/bar"
	"github.com/hsgiga/ptyline/internal/command"
	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/diagnostics"
	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/modules"
	"github.com/hsgiga/ptyline/internal/proxy"
	"github.com/hsgiga/ptyline/internal/pty"
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/runtimeenv"
	"github.com/hsgiga/ptyline/internal/shellintegration"
	"github.com/hsgiga/ptyline/internal/snapshot"
	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/layout"
	"github.com/hsgiga/ptyline/internal/status/renderer"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// ptyDrainGrace bounds how long the ChildExited emitter waits for the PTY reader
// to drain the child's final output after the shell is reaped. A lingering
// grandchild can keep the slave open indefinitely, so this caps the wait.
const ptyDrainGrace = 100 * time.Millisecond

// userModEntry tracks a running user-defined module goroutine (exec or custom-time).
type userModEntry struct {
	cancel    context.CancelFunc
	configKey string             // serialized ModuleConfig; used to detect restarts
	exec      *execModuleRuntime // nil for custom-time modules
}

// modConfigKey returns a deterministic string that changes whenever any field
// of a ModuleConfig that affects the exec module's behavior changes. Reload
// compares this key to decide whether to restart the goroutine.
func modConfigKey(id string, mcfg config.ModuleConfig) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s|%d|%d|%v|%v|%v",
		id, mcfg.Command, mcfg.IntervalMS, mcfg.Format, mcfg.Separator,
		mcfg.MaxWidth, mcfg.TimeoutMS, mcfg.Env, mcfg.RefreshOnCommand, mcfg.RefreshOnCWD)
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
	// On a fresh install (default path, no file yet) seed an editable copy of the
	// sample config. Best-effort: a write failure still leaves built-in defaults.
	if opts.ConfigPath == "" {
		switch created, path, err := config.EnsureUserConfig(); {
		case err != nil:
			fmt.Fprintln(os.Stderr, "ptyline: could not write default config:", err)
		case created:
			fmt.Fprintln(os.Stderr, "ptyline: created default config at", path)
		}
	}
	handoff, present, err := parseHandoff()
	if present && err != nil {
		fmt.Fprintln(os.Stderr, "ptyline:", err)
		return 1
	}
	adopted := handoff != nil

	// In adopted mode the shell is already running on the handed-off PTY, and
	// exiting here would orphan it with no PTY reader — the user's session would
	// hang. A bad config must degrade to built-in defaults, never exit.
	cfg, cfgExplicitDisabled, err := config.Load(opts.ConfigPath)
	if err != nil {
		if !adopted {
			fmt.Fprintln(os.Stderr, "ptyline: config:", err)
			return 1
		}
		fmt.Fprintln(os.Stderr, "ptyline: config:", err, "— continuing with built-in defaults")
		cfg = config.Default()
		cfgExplicitDisabled = nil
	}

	cliOverlay := config.ResolveOverlayPath(opts.OverlayPath)
	var initProjectOverlay string
	if !opts.NoProjectPtyline {
		if cwd, err := os.Getwd(); err == nil {
			initProjectOverlay, _ = config.FindProjectConfig(cwd)
		}
	}
	resolvedCfg, err := config.ApplyOverlays(cfg, cfgExplicitDisabled, cliOverlay, initProjectOverlay)
	if err != nil {
		if !adopted {
			fmt.Fprintln(os.Stderr, "ptyline: overlay:", err)
			return 1
		}
		fmt.Fprintln(os.Stderr, "ptyline: overlay:", err, "— continuing without overlays")
		resolvedCfg = cfg
	}

	barRows := bar.BuildRows(resolvedCfg)
	area := reserved.Area{Edge: reserved.Bottom, Rows: uint16(len(barRows))}

	binID := captureBinaryIdentity()

	argv := resolveChild(opts.Child, resolvedCfg, profile)
	if adopted {
		argv = handoff.ChildArgv
	}

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
	if !adopted {
		_, _ = ctrl.Write([]byte(terminal.ClearScreen + terminal.CursorTo(1, 1)))
	}

	// --- Child PTY sized to rows-minus-reserved (spec §8.2). ---
	//
	// In adopted mode (re-exec handoff) the child is already running; we inherit
	// its PTY master fd and PID from the previous process image via PTYLINE_HANDOFF.
	var sup *pty.Supervisor
	if adopted {
		sup = pty.Adopt(handoff.PtyFD, handoff.ChildPID, area)
	} else {
		sup = pty.New(argv, area)
	}
	// execEnvNonce authenticates exec_env frames from the shell integration so a
	// program that only injects bytes into the stream (e.g. a malicious file cat)
	// cannot forge the environment we run exec-module commands with.
	//
	// Threat model: exec_env carries base64-encoded env vars (e.g. tokens for GH_TOKEN,
	// AWS_SECRET_ACCESS_KEY) that are passed to exec module subprocesses. A nonce in the
	// frame prevents injection: an attacker printing an OSC 777 exec_env=<payload> from
	// a file or command output cannot supply the nonce, so the frame is dropped.
	//
	// cwd is authenticated with the same nonce: it is not display-only — it sets the
	// working directory of exec-module subprocesses and the root for project .ptyline
	// discovery, so a forged cwd could redirect where commands run and which overlay
	// loads. command, exit_code, and env stay unauthenticated: they only feed the bar
	// display (worst case: a wrong command/branch shown), never command execution.
	//
	// In adopted mode the shell already holds the nonce from the previous image; reuse
	// it so OSC exec_env/cwd frames keep passing authentication.
	execEnvNonce := newNonce()
	if adopted {
		execEnvNonce = handoff.Nonce
	}
	sup.SetEnv("PTYLINE_ENV_NAMES", strings.Join(resolvedCfg.Modules["env"].Env, ","))
	sup.SetEnv("PTYLINE_EXEC_ENV_NAMES", strings.Join(execEnvNames(resolvedCfg), ","))
	sup.SetEnv("PTYLINE_NONCE", execEnvNonce)
	sup.SetEnv("PTYLINE_PID", strconv.Itoa(os.Getpid()))
	if !adopted {
		if err := sup.Start(pty.Size{Cols: size.Cols, Rows: size.Rows}); err != nil {
			fmt.Fprintln(os.Stderr, "ptyline: pty:", err)
			return 1
		}
	}

	// --- Event loop + ANSI/OSC filter. ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := event.NewBus(256)
	filter := proxy.NewAnsiFilter(area)
	filter.SetRows(size.Rows)
	diagState := diagnostics.New()
	openDebugLog(diagState)
	filter.SetDiagHandler(func(msg string) { diagState.RecordAnsiWarning(msg) })
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

	visuals, err := bar.VisualsFromConfig(resolvedCfg, colorMode(profile.Capabilities.Color), opts.ConfigPath, modules.ShellLabel(argv))
	if err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: theme:", err)
		return 1
	}
	newEngine := func(cols int) *layout.Engine {
		return layout.NewWithMinBlock(cols, resolvedCfg.Bar.MinBlockWidth)
	}
	var changeAnimating atomic.Bool
	configureRenderer := func(r *renderer.Renderer) {
		r.SetJustify(renderer.Justify(resolvedCfg.Bar.Justify))
		r.SetStyles(visuals.Styles)
		r.SetAnimations(bar.AnimationsFromConfig(resolvedCfg.Modules))
		r.SetTemplates(bar.TemplateSpecs(resolvedCfg))
		r.SetIcons(bar.IconSpecs(resolvedCfg))
		r.SetChangeFlag(&changeAnimating)
	}
	render := renderer.New(newEngine(int(size.Cols)), visuals.Theme)
	configureRenderer(render)
	// animState survives renderer recreations on resize/reload so in-progress
	// change pulses and glints are not interrupted.
	var animState *renderer.AnimationState
	var resizePending bool
	renderFn := func() []string { return bar.Render(render, state, barRows) }
	redraw := func() {
		if resizePending {
			return
		}
		state.Diagnostics = diagState.Snapshot()
		writer.RequestRedraw()
		alt := filter.AltActive()
		_ = writer.FlushBarFrameLazy(renderFn, alt)
		// If the frame was deferred by rate-limiting, schedule a one-shot redraw so
		// the bar is repainted as soon as the window expires without waiting for the
		// next user event or module update. It must be RedrawRequest, not Tick: Tick
		// advances the animation phase, so using it here would let high-rate deferred
		// flushes run the command animation far faster than its tick interval.
		if due := writer.PendingRedrawDue(alt); due > 0 {
			time.AfterFunc(due, func() { bus.SendCtx(ctx, event.RedrawRequest{}) })
		}
	}
	altCoord := &altScreenCoordinator{
		ctrl: ctrl, sup: sup, writer: writer, state: &state, area: &area, redraw: redraw,
	}

	// projectOverlayPath tracks the active project .ptyline so we skip rebuilds
	// when cwd changes but the nearest overlay is the same file.
	var projectOverlayPath atomic.Value
	projectOverlayPath.Store(initProjectOverlay)

	// projectConfigCache avoids walking parent directories on every cwd change.
	// Keyed by cwd; cleared on reload so stale entries don't survive a config change.
	// Accessed only from the event loop goroutine — no locking needed.
	projectConfigCache := map[string]string{}

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
		probeMods    *probeModManager
	)
	var gitRefreshing atomic.Bool
	var gitPending atomic.Bool
	var execEnvMu sync.RWMutex
	execEnv := map[string]string{}

	// execEnvFor resolves a module's env patterns (exact names or GH_* prefixes)
	// against the shell's last reported snapshot.
	execEnvFor := func(patterns []string) []string {
		execEnvMu.RLock()
		defer execEnvMu.RUnlock()
		var env []string
		for name, value := range execEnv {
			for _, pattern := range patterns {
				if envNameMatches(name, pattern) {
					env = append(env, name+"="+value)
					break
				}
			}
		}
		return env
	}

	execDeps := &execRuntimeDeps{
		env: execEnvFor,
		cwd: func() string { s, _ := cwdHolder.Load().(string); return s },
	}
	userMods := newUserModSet(ctx, scheduler, &state, bus, execDeps)

	// refreshGitMod runs RefreshAll on the given git module off the event loop and
	// emits every sub-module snapshot. gm is passed explicitly so callers off the
	// event loop (the ticker goroutine) never read the shared gitMod variable,
	// which the loop rewrites on reload.
	refreshGitMod := func(gm *modules.Git, expectedCWD string) {
		gitPending.Store(true)
		if !gitRefreshing.CompareAndSwap(false, true) {
			return // worker already running; it will see gitPending when done
		}
		go func() {
			// done marks a clean hand-off; if we unwind without it (a panic in
			// RefreshAll/SendCtx) release the flag so git isn't wedged refreshing.
			done := false
			defer func() {
				if !done {
					gitRefreshing.Store(false)
				}
			}()
			for {
				for gitPending.CompareAndSwap(true, false) {
					// Re-read expectedCWD on each iteration from the caller's last value.
					// Since pendingCWD races, use cwdHolder as the freshest source.
					cwdAtStart, _ := cwdHolder.Load().(string)
					rctx, rcancel := context.WithTimeout(ctx, time.Second)
					snaps := gm.RefreshAll(rctx)
					rcancel()
					// Discard stale results: if the cwd changed while we were running,
					// keep pending so the loop runs once more with the current directory.
					if expectedCWD != "" {
						if cur, _ := cwdHolder.Load().(string); cur != cwdAtStart {
							gitPending.Store(true)
							continue
						}
					}
					for _, snap := range snaps {
						bus.SendCtx(ctx, event.ModuleUpdated{ID: string(snap.ID), Snapshot: snap})
					}
				}
				gitRefreshing.Store(false)
				// A request may have arrived between the failed CAS above and clearing
				// the flag; re-acquire and loop, unless another caller already did.
				if !gitPending.Load() || !gitRefreshing.CompareAndSwap(false, true) {
					done = true
					return
				}
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

		// Probe-driven system modules ({load}, {cpu}, …) share one lifecycle
		// manager that probes, schedules, and reconciles them on reload. Specs
		// come from the package registry (registerProbeMod).
		probeMods = newProbeModManager(ctx, scheduler, probeModDeps{
			cwd: func() string { s, _ := cwdHolder.Load().(string); return s },
		}, probeModRegistry)
		probeMods.Reconcile(resolvedCfg)

		// User-defined exec and custom-time modules.
		userMods.Reconcile(resolvedCfg)
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
			// Emit the initial snapshot off the event loop (as exec modules do) so a
			// reload triggered by `cd` never stalls terminal I/O for up to the git
			// timeout while RefreshAll runs.
			refreshGitMod(gitMod, "")
		}

		// Probe-driven system modules: one call starts/stops/restarts them all to
		// match the new config (enabled toggles, interval/format changes).
		probeMods.Reconcile(resolvedCfg)

		// User-defined exec and custom-time modules: diff by ID + full config key.
		userMods.Reconcile(resolvedCfg)

		// Rebuild cmdTracker so animation/threshold config from the new resolvedCfg
		// takes effect immediately (it is only read/written from the event loop).
		cmdTracker = command.NewTracker(resolvedCfg.Modules["command"])

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

	// as bundles pointers to the mutable variables captured by closures so that
	// reloadConfig can be a testable method on appState rather than a 40-line
	// closure. The pointers share the same backing storage as the locals above,
	// so existing closures continue to read/write the local variables directly
	// while the method updates them through the pointers (ARCHITECTURE.md §A1).
	as := &appState{
		cfg:                 cfg,
		cfgExplicitDisabled: cfgExplicitDisabled,
		cliOverlay:          cliOverlay,
		shell:               modules.ShellLabel(argv),
		resolvedCfg:         &resolvedCfg,
		area:                &area,
		barRows:             &barRows,
		visuals:             &visuals,
		render:              &render,
		animState:           &animState,
		projectOverlayPath:  &projectOverlayPath,
		projectConfigCache:  &projectConfigCache,
		diagState:           diagState,
		writer:              writer,
		filter:              filter,
		ctrl:                ctrl,
		sup:                 sup,
		profile:             profile,
		opts:                opts,
		termState:           &state,
		newEngine:           newEngine,
		configureRenderer:   configureRenderer,
		updateModules:       updateModules,
	}

	resizeDebouncer := proxy.NewResizeDebouncer(proxy.ResizeCommitDelay)
	resizeDebouncer.Start(ctx, bus)
	var pendingRefreshCommand string
	// activeRevealTimer fires one Tick after the command-appearance grace so the
	// tracker can reveal a still-running command even when no animation ticker is
	// active. A command that finishes first is never shown (see Tracker grace).
	var activeRevealTimer *time.Timer
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
			altCoord.FlushPending()
			return nil
		},
		// WriteOutputFramed handles child output that erased the reserved rows: it
		// writes the output and repaints the bar in one synchronized frame so the
		// bar never blinks blank (spec §8.4).
		WriteOutputFramed: func(data []byte) error {
			if err := writer.WriteChildFrame(data, renderFn(), filter.AltActive()); err != nil {
				return err
			}
			if len(data) > 0 {
				if changed := cmdTracker.RecordOutput(&state.Shell); changed {
					state.ActiveCommandAnimating = true
				}
			}
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
			animState = render.TakeAnimationState()
			render = renderer.NewWithState(newEngine(int(cols)), visuals.Theme, animState)
			configureRenderer(render)
			if alt {
				altCoord.MarkResizedDuringAlt()
				_ = sup.ResizeFull(pty.Size{Cols: cols, Rows: rows})
				ctrl.ResetScrollRegion()
				_, _ = ctrl.Write([]byte(terminal.ShowCursor))
				return
			}
			_ = sup.Resize(pty.Size{Cols: cols, Rows: rows})
			// Re-establish the scroll region. On Linux/WSL this preserves the cursor
			// (SaveCursor/RestoreCursor) so a resize/split does not jump the cursor to
			// the last line; on macOS it pins the cursor to the last child row because
			// the OS clamps it into the bar on shrink. See reapplyScrollRegionAfterResize.
			reapplyScrollRegionAfterResize(ctrl, terminal.Size{Cols: cols, Rows: rows}, area)
			_, _ = ctrl.Write([]byte(terminal.ShowCursor))
		},
		ShellMeta: func(key, value string) {
			if key == shellintegration.KeyCWD {
				// cwd is authenticated: it sets exec modules' working directory and
				// the root for project .ptyline discovery, so a frame without the
				// session nonce (a forged OSC 777) is dropped rather than applied.
				cwd, ok := stripNonce(value, execEnvNonce)
				if !ok {
					return
				}
				value = cwd
			}
			state.ApplyShellMeta(key, value)
			if key == shellintegration.KeyCommand && value != "" {
				pendingRefreshCommand = state.Shell.LastCommand
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
				probeMods.OnCWDChange()
				// Modules with refresh_on_cwd re-run right when the directory changes.
				// They run from the shell's cwd, so a `mise exec`/`direnv exec`-wrapped
				// command sees the new directory's environment and the bar updates
				// immediately — cwd arrives without the one-prompt lag that mirrored env
				// has under mise.
				for _, m := range userMods.ExecModules() {
					if m.refreshOnCWD {
						m.refresh(ctx, bus)
					}
				}
				if !opts.NoProjectPtyline {
					newPath, cached := projectConfigCache[state.Shell.CWD]
					if !cached {
						newPath, _ = config.FindProjectConfig(state.Shell.CWD)
						projectConfigCache[state.Shell.CWD] = newPath
					}
					as.reloadConfig(newPath, false)
				}
			}
			if key == shellintegration.KeyEnv {
				state.UpdateModule(status.ModuleSnapshot{
					ID:        "env",
					Value:     status.Text(value),
					UpdatedAt: time.Now(),
				})
			}
			if key == shellintegration.KeyExecEnv {
				// A valid frame is the shell's complete current snapshot, so replace
				// the map wholesale — variables unset since the last prompt vanish.
				// A nil result (missing/forged nonce) leaves state untouched.
				if snapshot := parseExecEnv(value, execEnvNonce); snapshot != nil {
					execEnvMu.Lock()
					changed := changedEnvNames(execEnv, snapshot)
					execEnv = snapshot
					execEnvMu.Unlock()
					// Refresh right when the mirrored environment actually changes
					// (e.g. cd into a mise/direnv directory) instead of waiting for
					// the module's interval, so the bar reflects the new context
					// immediately and reliably. Scoped to modules that mirror a
					// changed variable, and only on change, so it never storms.
					if len(changed) > 0 {
						for _, m := range userMods.ExecModules() {
							if m.mirrorsAny(changed) {
								m.refresh(ctx, bus)
							}
						}
					}
				}
			}
			if key == shellintegration.KeyExitCode {
				if shouldRefreshAfterExit(value, pendingRefreshCommand, state.Shell.LastCommand) {
					for _, m := range userMods.ExecModules() {
						m.refreshAfterCommand(ctx, bus, pendingRefreshCommand)
					}
				}
				pendingRefreshCommand = ""
			}
			if snap := cmdTracker.ApplyShellMeta(key, &state); snap != nil {
				state.UpdateModule(*snap)
			}
			if key == shellintegration.KeyCommand {
				if activeRevealTimer != nil {
					activeRevealTimer.Stop()
					activeRevealTimer = nil
				}
				if state.Shell.ActiveCommand != "" {
					if delay := cmdTracker.ActiveShowDelay(); delay > 0 {
						activeRevealTimer = time.AfterFunc(delay, func() {
							bus.SendCtx(ctx, event.Tick{})
						})
					}
				}
			}
		},
		ModuleUpdated: func(_ string, snap snapshot.ModuleSnapshot) {
			state.UpdateModule(snap)
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
		Redraw:        redraw,
		InvalidateBar: writer.InvalidateBar,
		ScrollReset: func() {
			// ESC c / CSI ! p: filter already substituted a clear-child-area sequence;
			// now re-establish the scroll region so the bar rows stay protected.
			size := terminal.Size{Cols: state.Terminal.Cols, Rows: state.Terminal.Rows}
			ctrl.ApplyScrollRegion(size, area)
			writer.InvalidateBar()
		},
		Terminate: func(sig string) { _ = sup.TerminateGroup(sig) },
		ConfigReload: func() {
			// If the binary on disk has been replaced, re-exec in place so the new
			// binary takes over without clearing the screen. reexecSelf never returns
			// on success (it replaced the process image). On failure fall through to
			// the regular config reload below.
			if changed, err := binID.changed(); err != nil {
				diagState.RecordConfigWarning(fmt.Sprintf("binary check: %v", err))
			} else if changed {
				if filter.AltActive() {
					diagState.RecordConfigWarning(
						"binary updated, but alt-screen app is active; re-exec deferred — run --reload again after exiting",
					)
				} else if err := reexecSelf(binID.path, ctrl, sup, execEnvNonce, argv); err != nil {
					diagState.RecordConfigWarning(fmt.Sprintf("re-exec: %v (reloading config instead)", err))
				} else {
					return // reexecSelf succeeded; process image replaced — unreachable
				}
			}
			newBase, newBaseDisabled, err := config.Load(opts.ConfigPath)
			if err != nil {
				diagState.RecordConfigWarning(fmt.Sprintf("reload %s: %v", opts.ConfigPath, err))
				return // keep running on bad config
			}
			prevCfg := as.cfg
			prevDisabled := as.cfgExplicitDisabled
			as.cfg = newBase
			as.cfgExplicitDisabled = newBaseDisabled
			currentPath, _ := as.projectOverlayPath.Load().(string)
			if !as.reloadConfig(currentPath, true) {
				as.cfg = prevCfg // ApplyOverlays failed: restore base to stay consistent
				as.cfgExplicitDisabled = prevDisabled
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
	if adopted {
		// The adopted PTY was sized by the previous image; re-apply the current
		// bar geometry in case the config changed the bar height. TIOCSWINSZ
		// notifies the shell via SIGWINCH automatically.
		_ = sup.Resize(pty.Size{Cols: size.Cols, Rows: size.Rows})
	}
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
