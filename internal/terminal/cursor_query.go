package terminal

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// cprGraceDuration bounds how long PositionQuery stays armed with no match
// before force-disarming, so a terminal that never answers DSR doesn't leave
// every future stdin chunk scanned for a cursor-position reply forever.
const cprGraceDuration = 2 * time.Second

// CursorPos is a 1-based terminal cursor position, as reported by a DSR
// (Device Status Report) reply: CSI row;col R.
type CursorPos struct {
	Row, Col uint16
}

// PositionQuery arms and matches a single outstanding DSR (CSI 6n) reply out
// of the real terminal's raw input stream.
//
// It exists because a resize handler cannot safely wait for a DSR reply by
// receiving from the event bus: that handler runs synchronously from inside
// the bus's own consumer loop, so the loop can never advance to deliver the
// reply while the handler is still waiting for it (self-deadlock). Filter
// must instead run directly on the goroutine that reads the real terminal's
// raw input, upstream of the event bus entirely, as an independent
// side-channel — see internal/app/app.go's stdin reader wiring and
// reapplyScrollRegionAfterShrink in internal/app/resize_scroll.go.
//
// Safe for concurrent use: Arm (and the timeout/select that follows it) is
// expected to run on one goroutine while Filter runs on another.
type PositionQuery struct {
	mu            sync.Mutex
	armed         bool
	buf           []byte
	ch            chan CursorPos
	grace         *time.Timer
	graceDuration time.Duration // overridable in tests; zero means cprGraceDuration
}

// Arm prepares to capture the next DSR reply and returns the channel it will
// be delivered on (buffered, capacity 1). Only one query may be outstanding
// at a time; a second Arm supersedes the first — its reply, if it ever
// arrives, is simply never read from the superseded channel.
func (q *PositionQuery) Arm() <-chan CursorPos {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.grace != nil {
		q.grace.Stop()
	}
	q.buf = nil
	q.armed = true
	q.ch = make(chan CursorPos, 1)
	d := q.graceDuration
	if d == 0 {
		d = cprGraceDuration
	}
	q.grace = time.AfterFunc(d, q.graceExpired)
	return q.ch
}

func (q *PositionQuery) graceExpired() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.armed = false
	q.buf = nil
}

// Filter scans data for a DSR reply while armed, stripping it so it never
// reaches the child PTY as garbage keystrokes, and returns the bytes that
// should still be forwarded. It is a no-op passthrough when not armed (the
// overwhelmingly common case). Any complete CSI sequence encountered while
// armed that is NOT a cursor-position reply (arrow keys, function keys,
// focus events, ...) passes through unchanged and leaves the query armed;
// an incomplete sequence at the end of data is buffered for the next call.
func (q *PositionQuery) Filter(data []byte) []byte {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.armed {
		return data
	}
	if len(q.buf) > 0 {
		data = append(q.buf, data...)
		q.buf = nil
	}

	out := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		if !q.armed {
			out = append(out, data[i:]...)
			return out
		}
		if data[i] != escByte {
			out = append(out, data[i])
			i++
			continue
		}
		if i+1 >= len(data) {
			q.buf = append(q.buf, data[i:]...)
			return out
		}
		if data[i+1] != '[' {
			out = append(out, data[i])
			i++
			continue
		}
		n, complete := scanCSI(data[i:])
		if !complete {
			q.buf = append(q.buf, data[i:]...)
			return out
		}
		seq := data[i : i+n]
		if pos, ok := parseCPR(seq); ok {
			select {
			case q.ch <- pos:
			default:
			}
			if q.grace != nil {
				q.grace.Stop()
			}
			q.armed = false
			q.buf = nil
		} else {
			out = append(out, seq...)
		}
		i += n
	}
	return out
}

const escByte = 0x1b

// scanCSI delimits a complete CSI sequence (params/intermediates 0x20-0x3f
// then a final byte 0x40-0x7e) starting at b[0:2] == "ESC [".
func scanCSI(b []byte) (n int, complete bool) {
	for j := 2; j < len(b); j++ {
		if b[j] >= 0x40 && b[j] <= 0x7e {
			return j + 1, true
		}
	}
	return 0, false
}

// parseCPR reports whether seq (a complete CSI sequence) is a DSR
// cursor-position reply — CSI row;col R — and if so its parsed 1-based
// row/col.
func parseCPR(seq []byte) (CursorPos, bool) {
	if len(seq) < 5 || seq[len(seq)-1] != 'R' {
		return CursorPos{}, false
	}
	params := string(seq[2 : len(seq)-1])
	parts := strings.SplitN(params, ";", 2)
	if len(parts) != 2 {
		return CursorPos{}, false
	}
	row, err1 := strconv.Atoi(parts[0])
	col, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || row < 0 || col < 0 {
		return CursorPos{}, false
	}
	return CursorPos{Row: uint16(row), Col: uint16(col)}, true
}
