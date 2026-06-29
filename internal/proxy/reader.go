package proxy

import (
	"context"
	"io"

	"github.com/hsgiga/ptyline/internal/event"
)

// StartReader pumps bytes from reader into bus, converting each read to an
// event using makeEvent. Stops when ctx is cancelled or reader returns an error.
// The returned channel is closed once the reader goroutine has exited (reader hit
// EOF/error or ctx was cancelled), so callers can sequence work after the stream
// has been fully drained — e.g. emitting ChildExited only after the last PtyOutput
// has been enqueued.
func StartReader(ctx context.Context, bus *event.Bus, reader io.Reader, makeEvent func([]byte) event.AppEvent) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 32*1024)
		for {
			n, err := reader.Read(buffer)
			if n > 0 {
				data := append([]byte(nil), buffer[:n]...)
				if !bus.SendCtx(ctx, makeEvent(data)) {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return done
}
