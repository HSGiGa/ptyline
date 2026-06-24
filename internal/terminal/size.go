package terminal

// Size is a terminal size in character cells.
type Size struct {
	Cols uint16
	Rows uint16
}

// QuerySize returns the current size of the real terminal (the controlling tty).
//
// TODO scaffold (plan 03): implement via golang.org/x/term.GetSize on the tty fd
// (TIOCGWINSZ). Returns a safe default until then.
func QuerySize() (Size, error) {
	return Size{Cols: 80, Rows: 24}, nil
}
