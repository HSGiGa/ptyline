// Package reserved models the screen region ptyline reserves for its status
// bar. All PTY sizing derives child height from terminal rows minus the reserved
// rows, so multi-line and panel modes work later without touching PTY logic.
package reserved

// Edge is the terminal edge the reserved area is anchored to.
type Edge int

const (
	// Bottom is the only edge supported by the MVP.
	Bottom Edge = iota
	Top
)

// Area describes the rows reserved for the status bar.
//
// MVP value is Area{Edge: Bottom, Rows: 1}. Multi-line / agent-panel modes
// (ARCHITECTURE.md §13) simply increase Rows.
type Area struct {
	Edge Edge
	Rows uint16
}

// Default returns the MVP reserved area: a single bottom row.
func Default() Area {
	return Area{Edge: Bottom, Rows: 1}
}

// ChildRows returns the height the child PTY should believe it has, given the
// real terminal height. It never returns less than 1. This is the single source
// of truth for the "rows - reserved" rule (spec §6).
func (a Area) ChildRows(terminalRows uint16) uint16 {
	if terminalRows <= a.Rows {
		return 1
	}
	return terminalRows - a.Rows
}

// BarTopRow returns the 1-based row index of the first status-bar row.
func (a Area) BarTopRow(terminalRows uint16) uint16 {
	return a.ChildRows(terminalRows) + 1
}
