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
	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/icons"
	"github.com/hsgiga/ptyline/internal/status/layout"
	"github.com/hsgiga/ptyline/internal/status/renderer"
	"github.com/hsgiga/ptyline/internal/status/theme"
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

	barRows := bar.BuildRows(cfg)
	area := reserved.Area{Edge: reserved.Bottom, Rows: uint16(len(barRows))}
	argv := resolveChild(opts.Child, cfg, profile)

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
	sup.SetEnv("PTYLINE_ENV_NAMES", strings.Join(cfg.Modules["env"].Env, ","))
	if err := sup.Start(pty.Size{Cols: size.Cols, Rows: size.Rows}); err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: pty:", err)
		return 1
	}

	// --- Event loop + ANSI/OSC filter. ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // stops module refresh tickers on every exit path
	bus := event.NewBus(256)
	filter := proxy.NewAnsiFilter(area, func(key, value string) {
		// TrySend (non-blocking): this callback is called synchronously from
		// filter.Filter inside the event loop, so a blocking Send would deadlock
		// if the bus buffer is full. Shell-meta events are rare enough that an
		// occasional drop is acceptable over a guaranteed deadlock.
		bus.TrySend(event.ShellMeta{Key: key, Value: value})
	})
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
	timeModule := modules.NewTime(cfg.Modules["time"].Format, moduleInterval(cfg.Modules["time"], time.Second))
	cmdTracker := command.NewTracker(cfg.Modules["command"])
	// Resolve the git branch icon through the icon preset: the Nerd-Font glyph
	// (U+E0A0) when icons.preset = "nerd-font", otherwise a plain-font branch
	// glyph (U+2387 "⎇") that renders without a Nerd Font. A true Nerd-Font check
	// is impossible at runtime, so the preset is the switch.
	branchIcon := icons.New(icons.Preset(cfg.Icons.Preset), cfg.Icons.Fallback).Icon("", "⎇")
	gitModule := modules.NewGit(moduleInterval(cfg.Modules["git"], 2*time.Second), time.Second, branchIcon, func() string {
		s, _ := cwdHolder.Load().(string)
		return s
	})
	envModule := modules.NewEnv(cfg.Modules["env"].Env)
	// Initial synchronous paint so the bar shows values immediately; the
	// scheduler then refreshes interval-driven modules (e.g. time) in the
	// background and feeds snapshots back through ModuleUpdated events.
	// sshBaseSnap is the env-based SSH snapshot (inbound SSH detection); it is
	// reused as the fallback when an outbound ssh_end event arrives.
	sshBaseSnap := modules.NewSSH().Refresh(nil)
	for _, module := range []status.Module{
		timeModule,
		modules.NewHostname(),
		modules.NewUser(),
		modules.NewRuntime(profile),
		modules.NewShell(argv),
		envModule,
	} {
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
	th := theme.Default(colorMode(profile.Capabilities.Color))
	render := renderer.New(layout.New(int(size.Cols)), th)
	render.SetAnimations(bar.AnimationsFromConfig(cfg.Modules))
	var resizePending bool
	redraw := func() {
		if resizePending {
			return
		}
		writer.RequestRedraw()
		_ = writer.FlushBarFrameLazy(func() []string {
			return bar.Render(render, state, barRows)
		})
	}
	// altCoord sequences the alt-screen entry/exit protocol (spec §11):
	// the filter records the transition; WriteOutput flushes it after bytes land.
	altCoord := &altScreenCoordinator{
		ctrl: ctrl, sup: sup, writer: writer, state: &state, area: area, redraw: redraw,
	}
	resizeDebouncer := proxy.NewResizeDebouncer(proxy.ResizeCommitDelay)
	resizeDebouncer.Start(ctx, bus)
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
			render = renderer.New(layout.New(int(cols)), th)
			render.SetAnimations(bar.AnimationsFromConfig(cfg.Modules))
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
			}
			if key == shellintegration.KeyEnv {
				state.UpdateModule(status.ModuleSnapshot{
					ID:        "env",
					Value:     status.Text(value),
					UpdatedAt: time.Now(),
				})
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
	bar.StartTicker(ctx, bus, cfg.Modules, cmdTracker.Animating())
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
