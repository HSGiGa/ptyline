module github.com/hsgiga/ptyline

go 1.25.0

toolchain go1.26.1

// External dependencies are intentionally not declared yet. The scaffold compiles
// against the standard library only; each implementation plan (docs/plans/NN-*.md)
// adds the packages it needs and runs `go mod tidy`. Expected future requires:
//
//	github.com/creack/pty        // Unix PTY backend (plan 04)
//	golang.org/x/term            // raw mode / terminal size (plan 03)
//	github.com/mattn/go-runewidth// display-width measurement (plan 09)
//	github.com/BurntSushi/toml   // config parsing (plan 02)
//	github.com/muesli/termenv    // capability detection / color (plan 10)

require (
	github.com/BurntSushi/toml v1.4.0
	github.com/creack/pty v1.1.24
	github.com/mattn/go-runewidth v0.0.24
	golang.org/x/term v0.31.0
)

require (
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
)
