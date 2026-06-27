package proxy

import (
	"github.com/hsgiga/ptyline/internal/event"
)

// Handlers contain the side effects owned by the application wiring. Loop keeps
// dispatch ordering serialized while platform-specific IO stays at the edge.
type Handlers struct {
	WriteInput    func([]byte) error
	WriteOutput   func([]byte) error
	ResizeRequest func(cols, rows uint16)
	ResizeCommit  func(cols, rows uint16)
	ShellMeta     func(key, value string)
	ModuleUpdated func(id string, snapshot any)
	Tick          func()
	Redraw        func()
	Terminate     func(signal string)
	ConfigReload  func()
}

// Loop is the single select-driven event loop. It multiplexes every input source
// via the event bus and is the only place that mutates terminal/PTY/bar state, so
// rendering stays predictable (spec §8.3, arch.md §4).
type Loop struct {
	bus    *event.Bus
	filter *AnsiFilter
	h      Handlers
}

// NewLoop wires the loop to the event bus and ANSI filter.
func NewLoop(bus *event.Bus, filter *AnsiFilter) *Loop {
	return &Loop{bus: bus, filter: filter}
}

// SetHandlers installs the application callbacks before Run.
func (l *Loop) SetHandlers(handlers Handlers) { l.h = handlers }

// Run consumes events until a ChildExited or TerminationSignal is seen, then
// returns the exit code. On a termination signal the code follows the shell
// convention 128+signo (SIGHUP 129, SIGINT 130, SIGTERM 143). Cleanup (terminal
// restore) is the caller's responsibility via defer so it runs even on panic
// (spec §15).
func (l *Loop) Run() (exitCode int, err error) {
	for ev := range l.bus.Events() {
		switch e := ev.(type) {
		case event.ChildExited:
			return e.Code, nil
		case event.TerminationSignal:
			if l.h.Terminate != nil {
				l.h.Terminate(e.Signal)
			}
			return terminationExitCode(e.Signal), nil
		case event.StdinInput:
			// Forward every byte verbatim, including Ctrl-D (EOT): the child PTY's
			// line discipline turns it into EOF for the foreground program, so an
			// interactive shell exits on its own (→ ChildExited). Intercepting it
			// here would break any program that reads stdin EOF (cat, REPLs, ssh).
			if l.h.WriteInput != nil {
				if err := l.h.WriteInput(e.Data); err != nil {
					return 1, err
				}
			}
		case event.PtyOutput:
			output := l.filter.Filter(e.Data)
			if l.h.WriteOutput != nil {
				if err := l.h.WriteOutput(output); err != nil {
					return 1, err
				}
			}
			l.applyFilterMeta()
			if l.h.Redraw != nil {
				l.h.Redraw()
			}
		case event.Resize:
			l.filter.SetRows(e.Rows)
			if l.h.ResizeRequest != nil {
				l.h.ResizeRequest(e.Cols, e.Rows)
			}
		case event.ResizeCommit:
			l.filter.SetRows(e.Rows)
			if l.h.ResizeCommit != nil {
				l.h.ResizeCommit(e.Cols, e.Rows)
			}
			if l.h.Redraw != nil {
				l.h.Redraw()
			}
		case event.ModuleUpdated:
			if l.h.ModuleUpdated != nil {
				l.h.ModuleUpdated(e.ID, e.Snapshot)
			}
			if l.h.Redraw != nil {
				l.h.Redraw()
			}
		case event.Tick:
			if l.h.Tick != nil {
				l.h.Tick()
			}
			if l.h.Redraw != nil {
				l.h.Redraw()
			}
		case event.ConfigReloadRequested:
			if l.h.ConfigReload != nil {
				l.h.ConfigReload()
			}
			if l.h.Redraw != nil {
				l.h.Redraw()
			}
		}
	}
	return 0, nil
}

func (l *Loop) applyFilterMeta() {
	if l.h.ShellMeta == nil {
		_ = l.filter.DrainMeta()
		return
	}
	for _, meta := range l.filter.DrainMeta() {
		l.h.ShellMeta(meta.Key, meta.Value)
	}
}

// terminationExitCode maps a termination-signal name to the conventional
// 128+signo exit code (spec §8.2). The producer sends canonical "SIGxxx" tokens.
func terminationExitCode(signal string) int {
	switch signal {
	case "SIGHUP":
		return 129
	case "SIGINT":
		return 130
	case "SIGTERM":
		return 143
	default:
		return 143
	}
}
