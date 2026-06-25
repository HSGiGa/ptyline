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
	"sync/atomic"
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

	barRows := buildBarRows(cfg)
	area := reserved.Area{Edge: reserved.Bottom, Rows: uint16(len(barRows))}
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
	top, count := barGeometry(area, size.Rows, len(barRows))
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
	timeModule := modules.NewTime(cfg.Modules["time"].Format, time.Second)
	// Resolve the git branch icon through the icon preset: the Nerd-Font glyph
	// (U+E0A0) when icons.preset = "nerd-font", otherwise a plain-font branch
	// glyph (U+2387 "⎇") that renders without a Nerd Font. A true Nerd-Font check
	// is impossible at runtime, so the preset is the switch.
	branchIcon := icons.New(icons.Preset(cfg.Icons.Preset), cfg.Icons.Fallback).Icon("", "⎇")
	gitModule := modules.NewGit(2*time.Second, time.Second, branchIcon, func() string {
		s, _ := cwdHolder.Load().(string)
		return s
	})
	// Initial synchronous paint so the bar shows values immediately; the
	// scheduler then refreshes interval-driven modules (e.g. time) in the
	// background and feeds snapshots back through ModuleUpdated events.
	for _, module := range []status.Module{timeModule, modules.NewHostname()} {
		state.UpdateModule(module.Refresh(nil))
	}
	scheduler := status.NewScheduler(func(snap status.ModuleSnapshot) {
		bus.Send(event.ModuleUpdated{ID: string(snap.ID), Snapshot: snap})
	})
	th := theme.Default(colorMode(profile.Capabilities.Color))
	render := renderer.New(layout.New(int(size.Cols)), th)
	var resizePending bool
	redraw := func() {
		if resizePending {
			return
		}
		writer.RequestRedraw()
		_ = writer.FlushBarFrame(renderBar(render, state, barRows))
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
	resizeRequests := make(chan terminal.Size, 1)
	go func() {
		var (
			timer   *time.Timer
			timerC  <-chan time.Time
			pending terminal.Size
		)
		stopTimer := func() {
			if timer == nil {
				return
			}
			if !timer.Stop() {
				select {
				case <-timerC:
				default:
				}
			}
			timer = nil
			timerC = nil
		}
		resetTimer := func() {
			if timer == nil {
				timer = time.NewTimer(resizeCommitDelay)
				timerC = timer.C
				return
			}
			if !timer.Stop() {
				select {
				case <-timerC:
				default:
				}
			}
			timer.Reset(resizeCommitDelay)
			timerC = timer.C
		}
		defer stopTimer()
		for {
			select {
			case <-ctx.Done():
				return
			case size := <-resizeRequests:
				pending = size
				resetTimer()
			case <-timerC:
				bus.Send(event.ResizeCommit{Cols: pending.Cols, Rows: pending.Rows})
				stopTimer()
			}
		}
	}()
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
		ResizeRequest: func(cols, rows uint16) {
			if !resizePending {
				// Hide the cursor for the duration of the resize burst so it does
				// not visibly teleport while the terminal reflows and the child
				// repaints (tmux does the same around its redraws).
				_, _ = ctrl.Write([]byte(terminal.HideCursor))
			}
			resizePending = true
			select {
			case resizeRequests <- terminal.Size{Cols: cols, Rows: rows}:
			default:
				select {
				case <-resizeRequests:
				default:
				}
				resizeRequests <- terminal.Size{Cols: cols, Rows: rows}
			}
		},
		ResizeCommit: func(cols, rows uint16) {
			alt := filter.AltActive()
			resizePending = false
			// Clearing only on a committed grow avoids ghost bars during resize.
			if !alt && rows > state.Terminal.Rows {
				_ = writer.ClearBar()
			}
			state.Resize(cols, rows, alt)
			top, count := barGeometry(area, rows, len(barRows))
			writer.SetBarRows(top, count)
			render = renderer.New(layout.New(int(cols)), th)
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
			if key == "cwd" {
				cwdHolder.Store(state.Shell.CWD)
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
	scheduler.Start(ctx, gitModule, time.Second)
	// Paint git as soon as possible without blocking startup if git hangs.
	go func() {
		rctx, rcancel := context.WithTimeout(ctx, time.Second)
		defer rcancel()
		bus.Send(event.ModuleUpdated{ID: "git", Snapshot: gitModule.Refresh(rctx)})
	}()
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
const resizeCommitDelay = 120 * time.Millisecond

func startSignals(bus *event.Bus) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for sig := range signals {
			switch sig {
			case syscall.SIGWINCH:
				if size, err := terminal.QuerySize(); err == nil {
					bus.Send(event.Resize{Cols: size.Cols, Rows: size.Rows})
				}
			case syscall.SIGHUP:
				bus.Send(event.TerminationSignal{Signal: "SIGHUP"})
			default: // SIGTERM
				bus.Send(event.TerminationSignal{Signal: "SIGTERM"})
			}
		}
	}()
}

// barRowSpec is one resolved bar row: its parsed blocks and gap/cap fill rune.
type barRowSpec struct {
	blocks []layout.Block
	fill   rune
}

// buildBarRows resolves the configured bar into rows. Multi-line [[bar.row]]
// entries take precedence; otherwise the single-line Format is one space-filled
// row. The number of rows is the reserved height.
func buildBarRows(cfg config.Config) []barRowSpec {
	if len(cfg.Bar.Rows) > 0 {
		rows := make([]barRowSpec, len(cfg.Bar.Rows))
		for i, rc := range cfg.Bar.Rows {
			fill := ' '
			if rc.Fill != "" {
				fill = []rune(rc.Fill)[0]
			}
			rows[i] = barRowSpec{blocks: layout.ParseFormat(rc.Format), fill: fill}
		}
		return rows
	}
	return []barRowSpec{{blocks: layout.ParseFormat(cfg.Bar.Format), fill: ' '}}
}

// renderBar renders every bar row to a line, top to bottom.
func renderBar(render *renderer.Renderer, st status.StatusState, rows []barRowSpec) []string {
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = render.RenderRow(st, row.blocks, row.fill).Line
	}
	return lines
}

// barGeometry returns the 1-based first bar row and how many of the `want` rows
// actually fit above the child area; on a short terminal the bottom rows are
// dropped so the bar never paints past the last row (spec §15).
func barGeometry(area reserved.Area, rows uint16, want int) (top uint16, count int) {
	child := area.ChildRows(rows)
	top = child + 1
	count = int(rows) - int(child)
	if count > want {
		count = want
	}
	if count < 0 {
		count = 0
	}
	return top, count
}

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
