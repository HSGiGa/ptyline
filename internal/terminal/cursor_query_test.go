package terminal

import (
	"testing"
	"time"
)

// Unarmed is the overwhelmingly common state: Filter must be a pure
// passthrough, including bytes that would otherwise look like a CPR reply.
func TestPositionQueryUnarmedPassthrough(t *testing.T) {
	q := &PositionQuery{}
	in := []byte("hello\x1b[24;10Rworld")
	if got := string(q.Filter(in)); got != string(in) {
		t.Fatalf("Filter(unarmed) = %q, want unchanged %q", got, in)
	}
}

// Armed, a CPR reply is parsed, delivered on the channel, and stripped from
// the forwarded bytes; surrounding bytes pass through unchanged.
func TestPositionQueryArmedMatchAndStrip(t *testing.T) {
	q := &PositionQuery{}
	ch := q.Arm()
	out := q.Filter([]byte("abc\x1b[24;10Rdef"))
	if got, want := string(out), "abcdef"; got != want {
		t.Fatalf("Filter = %q, want %q", got, want)
	}
	select {
	case pos := <-ch:
		if pos != (CursorPos{Row: 24, Col: 10}) {
			t.Fatalf("pos = %+v, want {24 10}", pos)
		}
	default:
		t.Fatal("no position delivered on channel")
	}
}

// A CPR split across two Filter calls (e.g. a short pty read boundary) is
// buffered and matched once the final byte arrives.
func TestPositionQuerySplitAcrossCalls(t *testing.T) {
	q := &PositionQuery{}
	ch := q.Arm()
	out1 := q.Filter([]byte("abc\x1b[24;1"))
	out2 := q.Filter([]byte("0Rdef"))
	if got, want := string(out1)+string(out2), "abcdef"; got != want {
		t.Fatalf("Filter (split) = %q, want %q", got, want)
	}
	select {
	case pos := <-ch:
		if pos != (CursorPos{Row: 24, Col: 10}) {
			t.Fatalf("pos = %+v, want {24 10}", pos)
		}
	default:
		t.Fatal("no position delivered on channel")
	}
}

// A non-CPR CSI sequence encountered while armed (e.g. an arrow key) must
// pass through byte-for-byte, unmodified, and must NOT disarm the query —
// the real CPR reply could still be on the way in the same or a later chunk.
func TestPositionQueryNonCPRCSIPassesThroughAndStaysArmed(t *testing.T) {
	q := &PositionQuery{}
	ch := q.Arm()
	arrowUp := "\x1b[A"
	out := q.Filter([]byte(arrowUp))
	if got := string(out); got != arrowUp {
		t.Fatalf("Filter(arrow key) = %q, want unchanged %q", got, arrowUp)
	}
	select {
	case pos := <-ch:
		t.Fatalf("unexpected delivery for non-CPR sequence: %+v", pos)
	default:
	}
	// Still armed: a CPR arriving afterwards is still matched and stripped.
	out2 := q.Filter([]byte("\x1b[5;5R"))
	if got := string(out2); got != "" {
		t.Fatalf("Filter(CPR after arrow key) = %q, want empty (stripped)", got)
	}
	select {
	case pos := <-ch:
		if pos != (CursorPos{Row: 5, Col: 5}) {
			t.Fatalf("pos = %+v, want {5 5}", pos)
		}
	default:
		t.Fatal("no position delivered on channel")
	}
}

// After a match, the query is disarmed: a second Arm gets a fresh channel,
// and nothing leaks onto the old one.
func TestPositionQueryDisarmsAfterMatch(t *testing.T) {
	q := &PositionQuery{}
	ch1 := q.Arm()
	q.Filter([]byte("\x1b[1;1R"))
	<-ch1

	// Now unarmed: passthrough, even for CPR-shaped bytes.
	if got, want := string(q.Filter([]byte("\x1b[2;2R"))), "\x1b[2;2R"; got != want {
		t.Fatalf("Filter(after disarm) = %q, want unchanged %q", got, want)
	}

	ch2 := q.Arm()
	if ch2 == ch1 {
		t.Fatal("Arm returned the same channel after a prior match")
	}
	q.Filter([]byte("\x1b[3;3R"))
	select {
	case pos := <-ch2:
		if pos != (CursorPos{Row: 3, Col: 3}) {
			t.Fatalf("pos = %+v, want {3 3}", pos)
		}
	default:
		t.Fatal("no position delivered on second channel")
	}
	select {
	case pos := <-ch1:
		t.Fatalf("unexpected late delivery on superseded channel: %+v", pos)
	default:
	}
}

// If no reply ever arrives, the grace timer force-disarms so the filter
// doesn't stay armed (scanning every future stdin chunk) forever.
func TestPositionQueryGraceTimerDisarms(t *testing.T) {
	q := &PositionQuery{graceDuration: 20 * time.Millisecond}
	q.Arm()
	time.Sleep(50 * time.Millisecond)
	// Now disarmed: a CPR-shaped chunk must pass through unmodified instead
	// of being matched/stripped.
	in := "\x1b[9;9R"
	if got := string(q.Filter([]byte(in))); got != in {
		t.Fatalf("Filter(after grace expiry) = %q, want unchanged %q", got, in)
	}
}
