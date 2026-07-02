package app

import (
	"fmt"
	"os"
	"time"

	"github.com/hsgiga/ptyline/internal/diagnostics"
)

// openDebugLog opens the file named by $PTYLINE_DEBUG and wires it as the
// diagnostics logger. Messages are flushed as timestamped lines so the file
// can be tailed while ptyline is running. A missing/empty PTYLINE_DEBUG is a
// no-op. The file is opened for append so multiple sessions accumulate.
func openDebugLog(d *diagnostics.State) {
	path := os.Getenv("PTYLINE_DEBUG")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ptyline: PTYLINE_DEBUG: %v\n", err)
		return
	}
	write := func(tag, msg string) {
		fmt.Fprintf(f, "%s [%s] %s\n", time.Now().Format(time.RFC3339Nano), tag, msg)
	}
	write("start", fmt.Sprintf("ptyline pid=%d", os.Getpid()))
	d.SetLogger(func(tag, msg string) { write(tag, msg) })
}
