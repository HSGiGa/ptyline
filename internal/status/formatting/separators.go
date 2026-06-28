package formatting

import "strings"

// CollapseSeparators treats "|" as a conditional separator marker. Empty
// segments are dropped, and the configured separator is inserted only between
// visible segments.
func CollapseSeparators(text, separator string) string {
	if !strings.Contains(text, "|") {
		return strings.Join(strings.Fields(text), " ")
	}
	return JoinSegments(strings.Split(text, "|"), separator)
}

// JoinSegments joins non-empty display segments with separator. Whitespace inside
// a segment is normalized so placeholder gaps do not leak into the status bar.
func JoinSegments(segments []string, separator string) string {
	if separator == "" {
		separator = " | "
	}
	separator = strings.TrimSpace(separator)
	visible := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.Join(strings.Fields(segment), " ")
		if segment == "" {
			continue
		}
		visible = append(visible, segment)
	}
	return strings.Join(visible, " "+separator+" ")
}
