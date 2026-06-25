package modules

import (
	"strings"

	"github.com/hsgiga/ptyline/internal/status/width"
)

const DefaultActiveCommandMaxWidth = 40

// FormatActiveCommand renders the shell-supplied foreground command as plain
// module text. The shell integration has already rejected control characters.
func FormatActiveCommand(command, format string, maxWidth int) string {
	if command == "" {
		return ""
	}
	if format == "" {
		format = "{command}"
	}
	if maxWidth <= 0 {
		maxWidth = DefaultActiveCommandMaxWidth
	}
	text := strings.ReplaceAll(format, "{command}", command)
	return width.Truncate(text, maxWidth, "right")
}
