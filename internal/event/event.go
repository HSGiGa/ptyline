// Package event defines the internal event bus. Every input source (stdin, PTY
// output, resize, timer, shell-meta OSC, child exit, signals, future agent
// updates) is normalized into an AppEvent and consumed by the single event loop
// in package proxy.
//
// Designing around a typed event stream from day one means new sources can be
// added without rewriting the application core. See ARCHITECTURE.md.
package event

import "context"

// AppEvent is the closed set of events the loop reacts to. It is a sealed
// interface: implementers live in this package so the loop can exhaustively
// type-switch over them.
type AppEvent interface {
	isAppEvent()
}

// StdinInput carries bytes read from the real terminal, destined for the child.
type StdinInput struct{ Data []byte }

// PtyOutput carries bytes read from the child PTY, destined (after filtering)
// for the real terminal.
type PtyOutput struct{ Data []byte }

// Resize reports a new real-terminal size.
type Resize struct{ Cols, Rows uint16 }

// ResizeCommit reports the final real-terminal size after resize has settled.
type ResizeCommit struct{ Cols, Rows uint16 }

// Tick is the periodic status-refresh signal.
type Tick struct{}

// ShellMeta carries a parsed shell-integration message (cwd, exit code, …).
type ShellMeta struct {
	Key   string
	Value string
}

// ModuleUpdated reports that a module produced a new snapshot. The payload is
// kept as any to avoid a dependency cycle with package status; the loop asserts
// the concrete type.
type ModuleUpdated struct {
	ID       string
	Snapshot any
}

// ChildExited reports the child process exit code.
type ChildExited struct{ Code int }

// TerminationSignal reports SIGINT/SIGTERM/SIGHUP.
type TerminationSignal struct{ Signal string }

// ConfigReloadRequested is sent when SIGUSR1 is received.
type ConfigReloadRequested struct{}

func (StdinInput) isAppEvent()            {}
func (PtyOutput) isAppEvent()             {}
func (Resize) isAppEvent()                {}
func (ResizeCommit) isAppEvent()          {}
func (Tick) isAppEvent()                  {}
func (ShellMeta) isAppEvent()             {}
func (ModuleUpdated) isAppEvent()         {}
func (ChildExited) isAppEvent()           {}
func (TerminationSignal) isAppEvent()     {}
func (ConfigReloadRequested) isAppEvent() {}

// Bus is the fan-in channel of events. Producers send; the loop receives.
type Bus struct {
	ch chan AppEvent
}

// NewBus creates a buffered event bus.
func NewBus(buffer int) *Bus {
	return &Bus{ch: make(chan AppEvent, buffer)}
}

// Send enqueues an event, blocking when the buffer is full. The blocking send is
// the backpressure policy: a slow loop throttles its producers (notably high-rate
// PtyOutput) instead of dropping or reordering bytes.
func (b *Bus) Send(e AppEvent) { b.ch <- e }

// SendCtx enqueues an event but returns false immediately when ctx is cancelled,
// preventing goroutines from blocking on a bus whose consumer has already exited.
func (b *Bus) SendCtx(ctx context.Context, e AppEvent) bool {
	select {
	case b.ch <- e:
		return true
	case <-ctx.Done():
		return false
	}
}

// TrySend enqueues an event non-blocking. Returns false when the buffer is full.
// Use inside the event loop itself (e.g. filter callbacks) where blocking would
// deadlock because the loop is the sole consumer.
func (b *Bus) TrySend(e AppEvent) bool {
	select {
	case b.ch <- e:
		return true
	default:
		return false
	}
}

// Events exposes the receive side for the loop.
func (b *Bus) Events() <-chan AppEvent { return b.ch }

// Close shuts down the bus.
func (b *Bus) Close() { close(b.ch) }
