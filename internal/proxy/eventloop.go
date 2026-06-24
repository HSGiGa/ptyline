package proxy

import "github.com/hsgiga/ptyline/internal/event"

// Loop is the single select-driven event loop. It multiplexes every input source
// via the event bus and is the only place that mutates terminal/PTY/bar state, so
// rendering stays predictable (spec §8.3, arch.md §4).
type Loop struct {
	bus    *event.Bus
	filter *AnsiFilter
}

// NewLoop wires the loop to the event bus and ANSI filter.
func NewLoop(bus *event.Bus, filter *AnsiFilter) *Loop {
	return &Loop{bus: bus, filter: filter}
}

// Run consumes events until a ChildExited or TerminationSignal is seen, then
// returns the child exit code. Cleanup (terminal restore) is the caller's
// responsibility via defer so it runs even on panic (spec §15).
//
// TODO scaffold (plan 05): handle each event —
//
//	StdinInput        → write to child PTY
//	PtyOutput         → filter.Filter, write to stdout, schedule redraw
//	Resize            → resize PTY, reapply scroll region, redraw
//	Tick              → refresh modules, redraw
//	ShellMeta         → update StatusState
//	ChildExited       → return code
//	TerminationSignal → return 130/143
func (l *Loop) Run() (exitCode int, err error) {
	for ev := range l.bus.Events() {
		switch e := ev.(type) {
		case event.ChildExited:
			return e.Code, nil
		case event.TerminationSignal:
			return 130, nil
		default:
			_ = e // TODO scaffold: dispatch remaining events
		}
	}
	return 0, nil
}
