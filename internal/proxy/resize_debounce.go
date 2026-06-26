package proxy

import (
	"context"
	"time"

	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// ResizeCommitDelay is the settling window after the last SIGWINCH before a
// ResizeCommit event is emitted (spec §12).
const ResizeCommitDelay = 50 * time.Millisecond

// ResizeDebouncer coalesces a burst of resize events (e.g. while dragging the
// window edge) into a single ResizeCommit after a settling delay.
type ResizeDebouncer struct {
	delay time.Duration
	ch    chan terminal.Size
}

// NewResizeDebouncer creates a debouncer with the given settling delay.
func NewResizeDebouncer(delay time.Duration) *ResizeDebouncer {
	return &ResizeDebouncer{delay: delay, ch: make(chan terminal.Size, 1)}
}

// Send delivers a size to the debouncer, replacing any pending unprocessed size
// so the latest value always wins.
func (d *ResizeDebouncer) Send(size terminal.Size) {
	select {
	case d.ch <- size:
	default:
		select {
		case <-d.ch:
		default:
		}
		d.ch <- size
	}
}

// Start launches the debounce goroutine. The goroutine stops when ctx is
// cancelled. Call Start before calling Send.
func (d *ResizeDebouncer) Start(ctx context.Context, bus *event.Bus) {
	go func() {
		var (
			timer   *time.Timer
			timerC  <-chan time.Time
			pending terminal.Size
		)
		stopTimer := func() {
			if timer == nil {
				return
			}
			if !timer.Stop() {
				select {
				case <-timerC:
				default:
				}
			}
			timer = nil
			timerC = nil
		}
		resetTimer := func() {
			if timer == nil {
				timer = time.NewTimer(d.delay)
				timerC = timer.C
				return
			}
			if !timer.Stop() {
				select {
				case <-timerC:
				default:
				}
			}
			timer.Reset(d.delay)
			timerC = timer.C
		}
		defer stopTimer()
		for {
			select {
			case <-ctx.Done():
				return
			case size := <-d.ch:
				pending = size
				resetTimer()
			case <-timerC:
				bus.SendCtx(ctx, event.ResizeCommit{Cols: pending.Cols, Rows: pending.Rows})
				stopTimer()
			}
		}
	}()
}
