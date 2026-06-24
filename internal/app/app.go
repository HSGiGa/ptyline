// Package app wires the components together and owns the run sequence. It parses
// the CLI, loads config, detects the runtime, then constructs the terminal
// controller, PTY supervisor, ANSI filter, and event loop. It is the only place
// that knows how the pieces fit; the packages themselves stay decoupled.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/modules"
	"github.com/hsgiga/ptyline/internal/proxy"
	"github.com/hsgiga/ptyline/internal/pty"
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/runtimeenv"
	"github.com/hsgiga/ptyline/internal/shellintegration"
	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/layout"
	"github.com/hsgiga/ptyline/internal/status/renderer"
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

	area := reserved.Default() // {Bottom, 1} for the MVP (cfg.Bar.Height drives this later)
	argv := resolveChild(opts.Child, cfg, profile)

	// --- Terminal: enter raw mode; ALWAYS restore on the way out (spec §15). ---
	ctrl := terminal.New(os.Stdin, os.Stdout)
	if err := ctrl.Enter(); err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: terminal:", err)
		return 1
	}
	defer func() { _ = ctrl.Restore() }()

	size, _ := terminal.QuerySize()
	ctrl.ApplyScrollRegion(size, area)
	_, _ = ctrl.Write([]byte(terminal.ClearScreen + terminal.CursorTo(1, 1)))

	// --- Child PTY sized to rows-minus-reserved (spec §8.2). ---
	sup := pty.New(argv, area)
	if err := sup.Start(pty.Size{Cols: size.Cols, Rows: size.Rows}); err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: pty:", err)
		return 1
	}

	// --- Event loop + ANSI/OSC filter. ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // stops module refresh tickers on every exit path
	bus := event.NewBus(256)
	filter := proxy.NewAnsiFilter(area, func(key, value string) {
		bus.Send(event.ShellMeta{Key: key, Value: value})
	})
	filter.SetRows(size.Rows)
	loop := proxy.NewLoop(bus, filter)
	writer := proxy.NewTerminalWriter(os.Stdout)
	writer.SetBarRow(size.Rows)
	state := status.NewState()
	state.Resize(size.Cols, size.Rows, false)
	if cwd, err := os.Getwd(); err == nil {
		state.Shell.CWD = cwd
		state.UpdateModule(status.ModuleSnapshot{
			ID:        "cwd",
			Value:     status.Text(modules.AbbreviateHome(cwd, "")),
			UpdatedAt: time.Now(),
		})
	}
	timeModule := modules.NewTime(cfg.Modules["time"].Format, time.Second)
	// Initial synchronous paint so the bar shows values immediately; the
	// scheduler then refreshes interval-driven modules (e.g. time) in the
	// background and feeds snapshots back through ModuleUpdated events.
	for _, module := range []status.Module{timeModule, modules.NewHostname()} {
		state.UpdateModule(module.Refresh(nil))
	}
	scheduler := status.NewScheduler(func(snap status.ModuleSnapshot) {
		bus.Send(event.ModuleUpdated{ID: string(snap.ID), Snapshot: snap})
	})
	render := renderer.New(layout.New(int(size.Cols)))
	blocks := layout.ParseFormat(cfg.Bar.Format)
	redraw := func() {
		writer.RequestRedraw()
		_ = writer.FlushBarFrame(render.Render(state, blocks).Line)
	}
	// applyAlt runs the alternate-screen entry/exit procedure (spec §11). It must
	// run AFTER the triggering ?1049h/l bytes have been written to the terminal,
	// otherwise it operates on the wrong screen — on exit the bar/scroll-region
	// work would be clobbered when the terminal restores the normal screen. The
	// filter records the transition; WriteOutput applies it once the bytes flush.
	var pendingAlt *bool
	applyAlt := func(active bool) {
		writer.SetAltActive(active)
		state.Terminal.AlternateScreen = active
		if active {
			ctrl.ResetScrollRegion()
			_ = sup.ResizeFull(pty.Size{Cols: state.Terminal.Cols, Rows: state.Terminal.Rows})
			return
		}
		// Leaving alt: the terminal has just restored the normal screen and the
		// pre-alt cursor; re-establish the child size and the protected region,
		// then repaint the bar.
		_ = sup.Resize(pty.Size{Cols: state.Terminal.Cols, Rows: state.Terminal.Rows})
		ctrl.ApplyScrollRegion(terminal.Size{Cols: state.Terminal.Cols, Rows: state.Terminal.Rows}, area)
		redraw()
	}
	loop.SetHandlers(proxy.Handlers{
		WriteInput: func(data []byte) error { _, err := sup.PTY().Write(data); return err },
		WriteOutput: func(data []byte) error {
			if err := writer.WriteChild(data); err != nil {
				return err
			}
			writer.InvalidateBar()
			// The child bytes (including any ?1049h/l) have now reached the
			// terminal; run a pending alt-screen transition on the correct screen.
			if pendingAlt != nil {
				applyAlt(*pendingAlt)
				pendingAlt = nil
			}
			return nil
		},
		ShellMeta: func(key, value string) {
			state.ApplyShellMeta(key, value)
			if key == "cwd" {
				state.UpdateModule(status.ModuleSnapshot{
					ID:        "cwd",
					Value:     status.Text(modules.AbbreviateHome(state.Shell.CWD, "")),
					UpdatedAt: time.Now(),
				})
			}
		},
		ModuleUpdated: func(_ string, snapshot any) {
			if snap, ok := snapshot.(status.ModuleSnapshot); ok {
				state.UpdateModule(snap)
			}
		},
		Resize: func(cols, rows uint16) {
			alt := filter.AltActive()
			// Erase the bar from its previous row before moving it, otherwise a
			// grown terminal leaves the old bar behind as a ghost second line.
			_ = writer.ClearBar()
			state.Resize(cols, rows, alt)
			writer.SetBarRow(rows)
			render = renderer.New(layout.New(int(cols)))
			if alt {
				// Alt screen: the child owns every row; no bar region (spec §12).
				_ = sup.ResizeFull(pty.Size{Cols: cols, Rows: rows})
				ctrl.ResetScrollRegion()
				return
			}
			_ = sup.Resize(pty.Size{Cols: cols, Rows: rows})
			ctrl.ApplyScrollRegion(terminal.Size{Cols: cols, Rows: rows}, area)
		},
		Redraw:    redraw,
		Terminate: func(sig string) { _ = sup.TerminateGroup(sig) },
	})
	// The filter detects the transition mid-stream; defer the actual procedure to
	// WriteOutput so it runs after the ?1049h/l bytes have been written.
	filter.SetAltHandler(func(active bool) {
		a := active
		pendingAlt = &a
	})
	startReader(bus, os.Stdin, func(data []byte) event.AppEvent { return event.StdinInput{Data: data} })
	startReader(bus, sup.PTY(), func(data []byte) event.AppEvent { return event.PtyOutput{Data: data} })
	go func() { code, _ := sup.Wait(); bus.Send(event.ChildExited{Code: code}) }()
	startSignals(bus)
	scheduler.Start(ctx, timeModule, 2*time.Second)
	redraw()
	code, err := loop.Run()
	_ = writer.ClearBar()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ptyline:", err)
		return 1
	}
	return code
}

func startReader(bus *event.Bus, reader io.Reader, makeEvent func([]byte) event.AppEvent) {
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			n, err := reader.Read(buffer)
			if n > 0 {
				data := append([]byte(nil), buffer[:n]...)
				bus.Send(makeEvent(data))
			}
			if err != nil {
				return
			}
		}
	}()
}

// resizeDebounce coalesces a burst of SIGWINCH events (e.g. while dragging the
// window edge) into a single re-query + Resize (spec §12, plan 05).
const resizeDebounce = 40 * time.Millisecond

func startSignals(bus *event.Bus) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		var resizeTimer *time.Timer
		sendResize := func() {
			if size, err := terminal.QuerySize(); err == nil {
				bus.Send(event.Resize{Cols: size.Cols, Rows: size.Rows})
			}
		}
		for sig := range signals {
			switch sig {
			case syscall.SIGWINCH:
				if resizeTimer == nil {
					resizeTimer = time.AfterFunc(resizeDebounce, sendResize)
				} else {
					resizeTimer.Reset(resizeDebounce)
				}
			case syscall.SIGHUP:
				bus.Send(event.TerminationSignal{Signal: "SIGHUP"})
			default: // SIGTERM
				bus.Send(event.TerminationSignal{Signal: "SIGTERM"})
			}
		}
	}()
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
