// Package width measures the *display* width of strings in terminal cells. The
// renderer must use this, never byte length or rune count, because CJK and many
// emoji occupy two cells while combining marks occupy zero (spec §8.6, §8.10).
package width

import "github.com/mattn/go-runewidth"

// String returns the display width of s in terminal cells.
func String(s string) int {
	return runewidth.StringWidth(s)
}

// RuneWidth returns the display width of a single rune.
func RuneWidth(r rune) int {
	return runewidth.RuneWidth(r)
}

const ellipsis = "…"

// Truncate shortens s to at most max display cells, applying the given side
// ("left" | "right" | "middle" | "none"). When truncation occurs an ellipsis is
// inserted on the trimmed side, measured by display width (spec §8.6).
func Truncate(s string, max int, side string) string {
	if max <= 0 {
		return ""
	}
	if String(s) <= max {
		return s
	}
	if side == "none" {
		// Hard cut with no ellipsis.
		return cut(s, max, false)
	}
	ew := String(ellipsis)
	if max <= ew {
		// No room for content alongside the ellipsis; hard-cut.
		return cut(s, max, false)
	}
	switch side {
	case "left":
		// Keep the right portion: "…tail".
		return ellipsis + cut(s, max-ew, true)
	case "middle":
		left := (max - ew) / 2
		right := (max - ew) - left
		return cut(s, left, false) + ellipsis + cut(s, right, true)
	default: // "right"
		return cut(s, max-ew, false) + ellipsis
	}
}

// cut returns at most n display cells of s. When fromEnd is true it keeps the
// trailing cells; otherwise the leading cells. A double-width rune that would
// straddle the boundary is dropped rather than split.
func cut(s string, n int, fromEnd bool) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if fromEnd {
		used := 0
		i := len(runes)
		for i > 0 {
			w := RuneWidth(runes[i-1])
			if used+w > n {
				break
			}
			used += w
			i--
		}
		return string(runes[i:])
	}
	used := 0
	i := 0
	for i < len(runes) {
		w := RuneWidth(runes[i])
		if used+w > n {
			break
		}
		used += w
		i++
	}
	return string(runes[:i])
}

// Pad right-pads (align "left"), left-pads (align "right"), or centers s to width
// cells using spaces. If s is already wider than width it is returned unchanged.
func Pad(s string, width int, align string) string {
	gap := width - String(s)
	if gap <= 0 {
		return s
	}
	switch align {
	case "right":
		return spaces(gap) + s
	case "center":
		left := gap / 2
		return spaces(left) + s + spaces(gap-left)
	default: // "left"
		return s + spaces(gap)
	}
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
