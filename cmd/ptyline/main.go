// Command ptyline runs a child shell inside a PTY and reserves the last
// terminal row(s) for a configurable status bar. See docs/ARCHITECTURE-overview.md.
package main

import (
	"os"

	"github.com/hsgiga/ptyline/internal/app"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(app.Run(os.Args[1:], version))
}
