package app

import (
	"errors"
	"fmt"
	"strings"
)

const usage = `ptyline — a PTY wrapper with a configurable bottom status bar

Usage:
  ptyline [flags] [command [args...]]
  ptyline init <shell>

Flags:
  --config <path>          use a specific config file
  --config=<path>          use a specific config file
  --ptyline <path>         apply a visual overlay (.ptyline file or short name)
  --ptyline=<path>         same, equals form
  --no-project-ptyline     disable automatic project .ptyline discovery
  --reload                 reload config; re-execs in place if the binary was updated (uses $PTYLINE_PID)
  --version                print version and exit
  --help                   show this help

Examples:
  ptyline                       run the configured shell or $SHELL
  ptyline --ptyline compact     apply ~/.config/ptyline/compact.ptyline overlay
  ptyline -- zsh                run zsh inside the wrapper
  ptyline -- ssh host.example   run any command (everything after -- is the child)
  ptyline init bash             print the bash shell-integration script
  ptyline --reload              reload config without restarting ptyline
`

// options is the parsed CLI invocation.
type options struct {
	ConfigPath       string
	OverlayPath      string // --ptyline (short name or path)
	NoProjectPtyline bool   // --no-project-ptyline
	Reload           bool   // --reload: send SIGUSR1 to $PTYLINE_PID
	Child            []string
	InitShell        string
	ShowVersion      bool
	ShowHelp         bool
}

// parseArgs is a minimal hand-rolled parser. Flags precede the child command;
// everything after `--` (or the first non-flag) is the child argv (spec §14).
func parseArgs(args []string) (options, error) {
	var o options
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--":
			i++
			o.Child = append(o.Child, args[i:]...)
			return o, nil
		case "--version":
			o.ShowVersion = true
		case "--help", "-h":
			o.ShowHelp = true
		case "--config":
			if i+1 >= len(args) {
				return o, errors.New("--config requires a path")
			}
			i++
			o.ConfigPath = args[i]
		case "--ptyline":
			if i+1 >= len(args) {
				return o, errors.New("--ptyline requires a path or short name")
			}
			i++
			o.OverlayPath = args[i]
		case "--no-project-ptyline":
			o.NoProjectPtyline = true
		case "--reload":
			o.Reload = true
		case "init":
			if i+1 >= len(args) {
				return o, errors.New("init requires a shell name")
			}
			o.InitShell = args[i+1]
			return o, nil
		default:
			if path, ok := strings.CutPrefix(a, "--config="); ok {
				if path == "" {
					return o, errors.New("--config requires a path")
				}
				o.ConfigPath = path
				continue
			}
			if path, ok := strings.CutPrefix(a, "--ptyline="); ok {
				if path == "" {
					return o, errors.New("--ptyline requires a path or short name")
				}
				o.OverlayPath = path
				continue
			}
			if strings.HasPrefix(a, "-") {
				return o, fmt.Errorf("unknown flag %q", a)
			}
			// First non-flag token starts the child command.
			o.Child = append(o.Child, args[i:]...)
			return o, nil
		}
	}
	return o, nil
}
