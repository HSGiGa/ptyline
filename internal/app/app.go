// Package app wires the components together and owns the run sequence. It parses
// the CLI, loads config, detects the runtime, then constructs the terminal
// controller, PTY supervisor, ANSI filter, and event loop. It is the only place
// that knows how the pieces fit; the packages themselves stay decoupled.
package app

import (
	"fmt"
	"os"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/proxy"
	"github.com/hsgiga/ptyline/internal/pty"
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/runtimeenv"
	"github.com/hsgiga/ptyline/internal/shellintegration"
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

	// --- Child PTY sized to rows-minus-reserved (spec §8.2). ---
	sup := pty.New(argv, area)
	if err := sup.Start(pty.Size{Cols: size.Cols, Rows: size.Rows}); err != nil {
		fmt.Fprintln(os.Stderr, "ptyline: pty:", err)
		return 1
	}

	// --- Event loop + ANSI/OSC filter. ---
	bus := event.NewBus(256)
	filter := proxy.NewAnsiFilter(area, func(key, value string) {
		// TODO scaffold (plan 07): apply ShellMeta to StatusState.
		_, _ = key, value
	})
	filter.SetRows(size.Rows)
	loop := proxy.NewLoop(bus, filter)

	// TODO scaffold (plan 05): spawn producers that feed `bus` — stdin reader,
	// PTY reader, SIGWINCH/SIGTERM handlers, status ticker — then run the loop:
	//
	//	code, _ := loop.Run()
	//	return code
	_ = loop

	fmt.Fprintln(os.Stderr, "ptyline: pipeline not yet wired — see docs/plans/05-event-bus-and-loop.md")
	return 0
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
