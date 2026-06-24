// Package width measures the *display* width of strings in terminal cells. The
// renderer must use this, never byte length or rune count, because CJK and many
// emoji occupy two cells while combining marks occupy zero (spec §8.6, §8.10).
package width

import "unicode/utf8"

// String returns the display width of s in terminal cells.
//
// TODO scaffold (plan 09): replace the rune-count placeholder with
// github.com/mattn/go-runewidth (StringWidth), honoring the configured
// emoji-width policy and ambiguous-width handling.
func String(s string) int {
	return utf8.RuneCountInString(s)
}

// Truncate shortens s to at most max display cells, applying the given side.
// TODO scaffold (plan 09): implement left/right/middle truncation with an
// ellipsis, measured by display width.
func Truncate(s string, max int, _ string) string {
	if String(s) <= max {
		return s
	}
	return s
}
