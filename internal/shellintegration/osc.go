// Package shellintegration owns the generic OSC shell-integration protocol and
// serves the per-shell init script printed by `ptyline init <shell>`.
//
// The Go implementation is SHELL-AGNOSTIC: there is no code path per shell. It
// owns one OSC 777 parser and one ShellState updater (consumed by the proxy
// filter). A shell-specific integration is only a small embedded script template
// that emits the common protocol. The PTY wrapper itself needs no integration and
// works with any shell or command (bash, zsh, fish, nu, ssh, vim, …); absence of a
// hook never affects PTY startup, job control, or terminal I/O (spec §9, §24.1).
package shellintegration

import (
	_ "embed"
	"sort"
)

// Recognized OSC 777 keys (spec §9). The proxy filter whitelists exactly these,
// maps them to ShellMeta events, then to StatusState fields.
const (
	KeyCWD        = "cwd"
	KeyExitCode   = "exit_code"
	KeyDurationMS = "duration_ms"
	KeyCommand    = "command"
)

//go:embed templates/bash.sh
var bashTemplate string

//go:embed templates/zsh.sh
var zshTemplate string

//go:embed templates/fish.sh
var fishTemplate string

// templates maps a shell name to its embedded init script. Adding a shell is a
// new template file plus an entry here — no Go logic per shell.
var templates = map[string]string{
	"bash": bashTemplate,
	"zsh":  zshTemplate,
	"fish": fishTemplate,
}

// Script returns the integration script for a shell, emitted by
// `ptyline init <shell>`.
func Script(shell string) (string, bool) {
	s, ok := templates[shell]
	return s, ok
}

// Supported returns the sorted list of shells with an integration template.
func Supported() []string {
	names := make([]string, 0, len(templates))
	for name := range templates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
