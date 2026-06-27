// Package shellcolors parses the OSC 777 "colors" payload emitted by shell
// integration scripts and converts it into per-module style overrides. It is
// the translation layer between shell-specific color variable conventions (e.g.
// fish $fish_color_cwd) and ptyline's style.Style / theme token system.
package shellcolors

import (
	"strings"

	"github.com/hsgiga/ptyline/internal/status/style"
)

// elementToModule maps OSC 777 colors sub-key names to ptyline module IDs.
// Elements not listed here are silently ignored (forward-compatible).
var elementToModule = map[string]string{
	"cwd":     "cwd",
	"host":    "hostname",
	"status":  "exit_code",
	"command": "command",
}

// fishToBright maps fish's abbreviated bright-color names to the canonical
// named-color strings that theme.ResolveInPalette accepts.
var fishToBright = map[string]string{
	"brblack":   "brightblack",
	"brred":     "brightred",
	"brgreen":   "brightgreen",
	"bryellow":  "brightyellow",
	"brblue":    "brightblue",
	"brmagenta": "brightmagenta",
	"brcyan":    "brightcyan",
	"brwhite":   "brightwhite",
}

// ParseColors splits the semicolon-separated "element=value" pairs of the
// OSC 777 colors payload into a map. Malformed entries are skipped.
func ParseColors(value string) map[string]string {
	result := make(map[string]string)
	for _, pair := range strings.Split(value, ";") {
		i := strings.IndexByte(pair, '=')
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(pair[:i])
		val := strings.TrimSpace(pair[i+1:])
		if key != "" {
			result[key] = val
		}
	}
	return result
}

// ParseColorAttr parses a fish color attribute string such as "green --bold"
// or "brred" and returns the canonical color name (usable as a theme FG
// reference) and whether bold is requested. "normal" and "" produce color=""
// (terminal default fg). Unknown flags are silently ignored.
func ParseColorAttr(s string) (color string, bold bool) {
	for _, part := range strings.Fields(s) {
		switch part {
		case "--bold":
			bold = true
		case "normal":
			// terminal default — leave color empty
		default:
			if strings.HasPrefix(part, "--") {
				continue // --italics, --underline, --dim, --background=..., etc.
			}
			if canonical, ok := fishToBright[part]; ok {
				color = canonical
			} else {
				color = part // standard names ("green", "red", …) pass through
			}
		}
	}
	return
}

// ParseToStyles converts the OSC 777 colors payload into a map of
// module-ID → style.Style. Unknown elements are silently dropped so that
// future shell-side additions do not break older ptyline builds.
func ParseToStyles(value string) map[string]style.Style {
	elements := ParseColors(value)
	styles := make(map[string]style.Style, len(elements))
	for elem, colorAttr := range elements {
		moduleID, ok := elementToModule[elem]
		if !ok {
			continue
		}
		color, bold := ParseColorAttr(colorAttr)
		styles[moduleID] = style.Style{FG: color, Bold: bold}
	}
	return styles
}
