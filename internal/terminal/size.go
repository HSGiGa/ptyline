package terminal

import (
	"os"

	"golang.org/x/term"
)

// Size is a terminal size in character cells.
type Size struct {
	Cols uint16
	Rows uint16
}

// QuerySize returns the current size of the real terminal (the controlling tty).
// It queries the given fd via golang.org/x/term (TIOCGWINSZ).
func QuerySize() (Size, error) {
	return QuerySizeFd(int(os.Stdout.Fd()))
}

// QuerySizeFd returns the size of the terminal on the given file descriptor.
func QuerySizeFd(fd int) (Size, error) {
	cols, rows, err := term.GetSize(fd)
	if err != nil {
		return Size{Cols: 80, Rows: 24}, err
	}
	return Size{Cols: clampUint16(cols), Rows: clampUint16(rows)}, nil
}

func clampUint16(n int) uint16 {
	if n < 0 {
		return 0
	}
	if n > int(^uint16(0)) {
		return ^uint16(0)
	}
	return uint16(n)
}
