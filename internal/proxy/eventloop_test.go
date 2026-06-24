package proxy

import (
	"bytes"
	"testing"

	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/reserved"
)

// Ctrl-D (EOT) must ride through to the child PTY as an ordinary byte, never be
// intercepted by the wrapper — otherwise programs that read stdin EOF (cat, REPLs,
// ssh) would kill ptyline instead of receiving EOF.
func TestLoopForwardsCtrlD(t *testing.T) {
	bus := event.NewBus(2)
	bus.Send(event.StdinInput{Data: []byte{0x04}})
	bus.Send(event.ChildExited{Code: 0}) // drives the loop to exit

	var written bytes.Buffer
	terminated := false
	loop := NewLoop(bus, NewAnsiFilter(reserved.Default(), nil))
	loop.SetHandlers(Handlers{
		WriteInput: func(b []byte) error { written.Write(b); return nil },
		Terminate:  func(string) { terminated = true },
	})

	code, err := loop.Run()
	if err != nil || code != 0 {
		t.Fatalf("Run() = (%d, %v), want (0, nil)", code, err)
	}
	if terminated {
		t.Fatal("Ctrl-D must not terminate the wrapper")
	}
	if got := written.Bytes(); !bytes.Equal(got, []byte{0x04}) {
		t.Fatalf("WriteInput got %v, want [4]", got)
	}
}

// A termination signal exits with the conventional 128+signo code and invokes the
// Terminate handler with the canonical signal token.
func TestLoopTerminationExitCode(t *testing.T) {
	cases := map[string]int{"SIGHUP": 129, "SIGINT": 130, "SIGTERM": 143}
	for sig, want := range cases {
		bus := event.NewBus(1)
		bus.Send(event.TerminationSignal{Signal: sig})
		var got string
		loop := NewLoop(bus, NewAnsiFilter(reserved.Default(), nil))
		loop.SetHandlers(Handlers{Terminate: func(s string) { got = s }})
		code, err := loop.Run()
		if err != nil || code != want {
			t.Fatalf("%s: Run() = (%d, %v), want (%d, nil)", sig, code, err, want)
		}
		if got != sig {
			t.Fatalf("%s: Terminate got %q", sig, got)
		}
	}
}
