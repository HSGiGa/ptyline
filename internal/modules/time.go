// Package modules implements the built-in status modules. Each satisfies
// status.Module: it refreshes on its own interval (with a timeout for expensive
// ones) and the renderer reads only the cached snapshot (spec §8.7).
package modules

import (
	"context"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// Time renders the current time. MVP module (spec §18).
type Time struct {
	id       status.ModuleID
	Format   string // strftime-style, e.g. "%H:%M:%S"
	interval time.Duration
}

// NewTime creates a time module.
func NewTime(format string, interval time.Duration) *Time {
	return NewTimeWithID("time", format, interval)
}

// NewTimeWithID creates a time-backed module with a custom placeholder ID.
func NewTimeWithID(id string, format string, interval time.Duration) *Time {
	if format == "" {
		format = "%H:%M:%S"
	}
	return &Time{id: status.ModuleID(id), Format: format, interval: interval}
}

func (m *Time) ID() status.ModuleID     { return m.id }
func (m *Time) Interval() time.Duration { return m.interval }

// Refresh returns the formatted current time.
func (m *Time) Refresh(_ context.Context) status.ModuleSnapshot {
	now := time.Now()
	return status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(now.Format(goTimeLayout(m.Format))),
		UpdatedAt: now,
	}
}

// strftimeToGo maps the strftime conversion specifiers ptyline supports to Go's
// reference-time layout tokens. Day-of-year (%j) and similar specifiers Go's
// layout cannot express are intentionally absent and pass through verbatim.
var strftimeToGo = map[byte]string{
	'Y': "2006", 'y': "06", 'm': "01", 'd': "02", 'e': "_2",
	'H': "15", 'I': "03", 'M': "04", 'S': "05", 'p': "PM",
	'A': "Monday", 'a': "Mon", 'B': "January", 'b': "Jan",
	'T': "15:04:05", 'R': "15:04", 'Z': "MST", 'z': "-0700",
}

// goTimeLayout converts a strftime-style format into a Go time layout. `%%` is a
// literal percent; an unknown `%x` is emitted verbatim so the user notices rather
// than getting silently wrong output.
func goTimeLayout(format string) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			b.WriteByte(format[i])
			continue
		}
		i++
		switch c := format[i]; {
		case c == '%':
			b.WriteByte('%')
		case strftimeToGo[c] != "":
			b.WriteString(strftimeToGo[c])
		default:
			b.WriteByte('%')
			b.WriteByte(c)
		}
	}
	return b.String()
}
