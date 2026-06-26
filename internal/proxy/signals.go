//go:build unix

package proxy

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// StartSignals installs OS signal handlers and forwards SIGWINCH, SIGINT,
// SIGHUP, and SIGTERM to the bus as AppEvent values.
func StartSignals(ctx context.Context, bus *event.Bus) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		defer signal.Stop(signals)
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-signals:
				if !ok {
					return
				}
				switch sig {
				case syscall.SIGWINCH:
					if size, err := terminal.QuerySize(); err == nil {
						bus.SendCtx(ctx, event.Resize{Cols: size.Cols, Rows: size.Rows})
					}
				case syscall.SIGINT:
					bus.SendCtx(ctx, event.TerminationSignal{Signal: "SIGINT"})
				case syscall.SIGHUP:
					bus.SendCtx(ctx, event.TerminationSignal{Signal: "SIGHUP"})
				default: // SIGTERM
					bus.SendCtx(ctx, event.TerminationSignal{Signal: "SIGTERM"})
				}
			}
		}
	}()
}
