module github.com/hsgiga/ptyline

go 1.23

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
