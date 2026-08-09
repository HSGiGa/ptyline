package app

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/terminal"
)

// The resize path pins the cursor to the last child row only for the one
// combination where the terminal has already clamped it into the bar:
// a clamping terminal (Terminal.app) that shrank in rows. Every other case
// must preserve the cursor via SaveCursor/RestoreCursor, or the input line
// jumps to the bottom on resize.
func TestReapplyScrollRegionAfterResize(t *testing.T) {
	size := terminal.Size{Cols: 80, Rows: 30}
	area := reserved.Default() // 1 bar row → child bottom is 29
	preserve := terminal.SaveCursor + terminal.SetScrollRegion(1, 29) + terminal.RestoreCursor
	pin := terminal.SetScrollRegion(1, 29) + terminal.CursorTo(29, 1)

	tests := []struct {
		name           string
		shrank         bool
		clampsOnShrink bool
		want           string
	}{
		{name: "clamping terminal, shrink", shrank: true, clampsOnShrink: true, want: pin},
		{name: "clamping terminal, grow", shrank: false, clampsOnShrink: true, want: preserve},
		{name: "preserving terminal, shrink", shrank: true, clampsOnShrink: false, want: preserve},
		{name: "preserving terminal, grow", shrank: false, clampsOnShrink: false, want: preserve},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			ctrl := terminal.New(nil, &buf)
			reapplyScrollRegionAfterResize(ctrl, size, area, test.shrank, test.clampsOnShrink)
			if got := buf.String(); got != test.want {
				t.Errorf("wrote %q, want %q", got, test.want)
			}
		})
	}
}

// feedCPRReply repeatedly feeds a DSR reply into q until done is closed, so
// it lands regardless of the exact moment reapplyScrollRegionAfterShrink
// arms the query. Once matched, PositionQuery self-disarms, so further
// redundant feeds are harmless no-ops.
func feedCPRReply(q *terminal.PositionQuery, row, col uint16, done <-chan struct{}) {
	reply := []byte(fmt.Sprintf("\x1b[%d;%dR", row, col))
	go func() {
		ticker := time.NewTicker(200 * time.Microsecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				q.Filter(reply)
			}
		}
	}()
}

// reapplyScrollRegionAfterShrink queries the real cursor position rather than
// trusting SaveCursor/RestoreCursor: it must preserve when the reply shows
// the cursor genuinely inside the child area, pin when the reply shows it
// already sitting in the reserved bar (proof the terminal clamped it before
// ptyline reacted to the resize), and fall back to the existing
// clampsOnShrink heuristic when the terminal never replies or the context is
// canceled — so a terminal this fix doesn't confirm clamps can only behave
// as well as it did before.
func TestReapplyScrollRegionAfterShrink(t *testing.T) {
	size := terminal.Size{Cols: 80, Rows: 30}
	area := reserved.Default() // 1 bar row → child bottom is 29
	preserve := terminal.SaveCursor + terminal.SetScrollRegion(1, 29) + terminal.RestoreCursor
	pin := terminal.SetScrollRegion(1, 29) + terminal.CursorTo(29, 1)
	query := terminal.QueryCursorPosition

	t.Run("reply inside child area preserves", func(t *testing.T) {
		var buf bytes.Buffer
		ctrl := terminal.New(nil, &buf)
		posQuery := &terminal.PositionQuery{}
		done := make(chan struct{})
		feedCPRReply(posQuery, 15, 1, done)
		reapplyScrollRegionAfterShrink(context.Background(), ctrl, posQuery, size, area, true, noopTrace)
		close(done)
		if got, want := buf.String(), query+preserve; got != want {
			t.Errorf("wrote %q, want %q", got, want)
		}
	})

	t.Run("reply inside reserved bar pins", func(t *testing.T) {
		var buf bytes.Buffer
		ctrl := terminal.New(nil, &buf)
		posQuery := &terminal.PositionQuery{}
		done := make(chan struct{})
		feedCPRReply(posQuery, 30, 1, done)
		reapplyScrollRegionAfterShrink(context.Background(), ctrl, posQuery, size, area, true, noopTrace)
		close(done)
		if got, want := buf.String(), query+pin; got != want {
			t.Errorf("wrote %q, want %q", got, want)
		}
	})

	t.Run("no reply falls back to clampsOnShrink heuristic", func(t *testing.T) {
		orig := cursorQueryTimeout
		cursorQueryTimeout = 5 * time.Millisecond
		defer func() { cursorQueryTimeout = orig }()

		for _, test := range []struct {
			name           string
			clampsOnShrink bool
			want           string
		}{
			{"clamping terminal", true, query + pin},
			{"preserving terminal", false, query + preserve},
		} {
			t.Run(test.name, func(t *testing.T) {
				var buf bytes.Buffer
				ctrl := terminal.New(nil, &buf)
				posQuery := &terminal.PositionQuery{}
				reapplyScrollRegionAfterShrink(context.Background(), ctrl, posQuery, size, area, test.clampsOnShrink, noopTrace)
				if got := buf.String(); got != test.want {
					t.Errorf("wrote %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("canceled context falls back to clampsOnShrink heuristic", func(t *testing.T) {
		var buf bytes.Buffer
		ctrl := terminal.New(nil, &buf)
		posQuery := &terminal.PositionQuery{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		reapplyScrollRegionAfterShrink(ctx, ctrl, posQuery, size, area, true, noopTrace)
		if got, want := buf.String(), query+pin; got != want {
			t.Errorf("wrote %q, want %q", got, want)
		}
	})
}

func noopTrace(string, string) {}
