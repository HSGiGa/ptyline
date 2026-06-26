// Package shellintegration owns the generic OSC shell-integration protocol and
// serves the per-shell init script printed by `ptyline init <shell>`.
//
// The Go implementation is SHELL-AGNOSTIC: there is no code path per shell. It
// owns one OSC 777 protocol (keys + whitelist, below) consumed by the proxy
// filter and one ShellState updater. A shell-specific integration is only a small
// embedded script template that emits the common protocol; the registry reads the
// embedded `templates/` directory, so adding a shell is a new template file with
// ZERO Go edits. The PTY wrapper itself needs no integration and works with any
// shell or command; absence of a hook never affects PTY startup, job control, or
// terminal I/O (spec §9, §24.1).
package shellintegration

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

// Recognized OSC 777 keys (spec §9). This package is the single owner of the
// protocol whitelist — keyed by protocol key, never by shell. The proxy filter
// consumes it (see AllowedSet) rather than redefining it.
const (
	KeyCWD        = "cwd"
	KeyExitCode   = "exit_code"
	KeyDurationMS = "duration_ms"
	KeyCommand    = "command"
	KeySSHStart   = "ssh_start" // emitted by the ssh shell wrapper before connecting
	KeySSHEnd     = "ssh_end"   // emitted by the ssh shell wrapper after disconnecting
)

// Keys is the OSC 777 metadata whitelist, in canonical order.
var Keys = []string{KeyCWD, KeyExitCode, KeyDurationMS, KeyCommand, KeySSHStart, KeySSHEnd}

// AllowedSet returns the whitelist as a lookup set for the proxy filter.
func AllowedSet() map[string]bool {
	set := make(map[string]bool, len(Keys))
	for _, k := range Keys {
		set[k] = true
	}
	return set
}

//go:embed templates/*.sh
var templatesFS embed.FS

const templateDir = "templates"

// Script returns the integration script for a shell, emitted by
// `ptyline init <shell>`. The lookup is purely data-driven: it reads
// templates/<shell>.sh from the embedded FS, so no Go code names a shell.
func Script(shell string) (string, bool) {
	if shell == "" || strings.ContainsAny(shell, "/.\\") {
		return "", false // reject path separators / traversal
	}
	data, err := templatesFS.ReadFile(templateDir + "/" + shell + ".sh")
	if err != nil {
		return "", false
	}
	return string(data), true
}

// Supported returns the sorted list of shells with an embedded template,
// derived from the templates/ directory contents.
func Supported() []string {
	entries, err := fs.ReadDir(templatesFS, templateDir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".sh"); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
