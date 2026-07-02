package proxy

import (
	"strings"

	"github.com/hsgiga/ptyline/internal/shellintegration"
)

// OSC shell-integration protocol (spec §9, ARCHITECTURE.md §11.1). Messages arrive as:
//
//	OSC 777 ; key=value ST
//
// where ST is ESC \ (or BEL). The filter parses these strictly, updates state,
// and must never let them reach the real terminal — and must never treat a value
// as a command to execute (spec §17).
const (
	oscIntroducer = "\x1b]"  // ESC ]
	oscTerminator = "\x1b\\" // ESC \  (ST)
	oscShellCode  = "777"

	// maxOSCPayload bounds an OSC 777 payload; larger messages are discarded with
	// a diagnostic (spec §9, §16: "maximum OSC payload: 8 KiB").
	maxOSCPayload = 8 * 1024
)

// oscAllowedKeys is the MVP whitelist of OSC 777 metadata keys (spec §9). The
// whitelist's single owner is the shellintegration package (keyed by protocol
// key, never by shell); the filter consumes it here. Any other key is dropped
// with a diagnostic and never causes command execution.
var oscAllowedKeys = shellintegration.AllowedSet()

// parseOSC777 splits and validates a "key=value" payload. It rejects unknown
// keys and oversized payloads. Values with control characters are rejected
// except for the "command" key, where newlines and tabs are collapsed to spaces
// so multiline shell commands are tracked rather than dropped (spec §9).
// It is called from the filter's OSC branch (handleOSC); future agent events
// (agent.started/update/done, spec §24.5) reuse the same channel with structured
// keys.
func parseOSC777(payload string) (key, value string, ok bool) {
	if len(payload) > maxOSCPayload {
		return "", "", false
	}
	k, v, found := strings.Cut(payload, "=")
	if !found || !oscAllowedKeys[k] {
		return "", "", false
	}
	if k == shellintegration.KeyCommand {
		v = sanitizeCommandValue(v)
	} else if hasControlChars(v) {
		return "", "", false
	}
	return k, v, true
}

// sanitizeCommandValue collapses whitespace control characters (newlines, tabs,
// carriage returns) to spaces so multiline commands survive the OSC frame
// without dropping the entire event. Other C0/C1 control characters are stripped.
func sanitizeCommandValue(v string) string {
	var b strings.Builder
	b.Grow(len(v))
	for _, r := range v {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteByte(' ')
		case r < 0x20 || (r >= 0x7f && r <= 0x9f):
			// strip other control chars
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// hasControlChars reports whether s contains C0/C1 control characters, which are
// not permitted in OSC metadata values (spec §9).
func hasControlChars(s string) bool {
	for _, r := range s {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}
