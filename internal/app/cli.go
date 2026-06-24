package app

import "errors"

const usage = `ptyline — a PTY wrapper with a configurable bottom status bar

Usage:
  ptyline [flags] [command [args...]]
  ptyline init <shell>

Flags:
  --config <path>   use a specific config file
  --version         print version and exit
  --help            show this help

Examples:
  ptyline                       run the configured shell or $SHELL
  ptyline -- zsh                run zsh inside the wrapper
  ptyline -- ssh host.example   run any command (everything after -- is the child)
  ptyline init bash             print the bash shell-integration script
`

// options is the parsed CLI invocation.
type options struct {
	ConfigPath  string
	Child       []string
	InitShell   string
	ShowVersion bool
	ShowHelp    bool
}

// parseArgs is a minimal hand-rolled parser. Flags precede the child command;
// everything after `--` (or the first non-flag) is the child argv (spec §14).
//
// TODO scaffold (plan 11): harden parsing, support `--config=path`, and validate.
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
		case "init":
			if i+1 >= len(args) {
				return o, errors.New("init requires a shell name")
			}
			o.InitShell = args[i+1]
			return o, nil
		default:
			// First non-flag token starts the child command.
			o.Child = append(o.Child, args[i:]...)
			return o, nil
		}
	}
	return o, nil
}
