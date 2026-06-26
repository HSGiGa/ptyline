//go:build windows

package proxy

import (
	"context"

	"github.com/hsgiga/ptyline/internal/event"
)

// StartSignals is a stub for Windows (ConPTY backend is post-MVP, spec §19).
// On Unix the real implementation handles SIGWINCH/SIGINT/SIGHUP/SIGTERM.
func StartSignals(_ context.Context, _ *event.Bus) {}
