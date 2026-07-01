package modules

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/formatting"
	"github.com/hsgiga/ptyline/internal/status/width"
)

const (
	execStdoutLimit = 4096
	execStderrLimit = 4096
)

const defaultExecMaxWidth = 60

// execWaitDelay bounds how long cmd.Wait blocks after the deadline/cancel before
// the process is killed and its pipes are force-closed, so a command that spawns
// a backgrounded child holding stdout open cannot hang the refresh goroutine.
const execWaitDelay = 2 * time.Second

// Exec runs a shell command on a configurable interval and publishes the
// captured stdout as a module snapshot. It is the canonical "user-defined"
// provider: expensive work happens on its own goroutine with a timeout; the
// renderer reads only the cached snapshot (spec §8.7, §17).
type Exec struct {
	id        status.ModuleID
	command   string
	interval  time.Duration
	timeout   time.Duration
	format    string
	separator string
	maxWidth  int
	env       []string
}

// NewExec creates an Exec module. id is the bar placeholder name (e.g. "gh").
// format uses {stdout}, {stderr}, {exit_code} placeholders; an empty format
// defaults to "{stdout}". maxWidth <= 0 applies a default cap so a misbehaving
// command cannot overflow the status bar.
func NewExec(id, command string, interval, timeout time.Duration, format, separator string, maxWidth int) *Exec {
	if format == "" {
		format = "{stdout}"
	}
	if maxWidth <= 0 {
		maxWidth = defaultExecMaxWidth
	}
	return &Exec{id: status.ModuleID(id), command: command, interval: interval, timeout: timeout, format: format, separator: separator, maxWidth: maxWidth}
}

func (m *Exec) ID() status.ModuleID     { return m.id }
func (m *Exec) Interval() time.Duration { return m.interval }
func (m *Exec) Timeout() time.Duration  { return m.timeout }

func (m *Exec) WithEnv(names []string) *Exec {
	m.env = append([]string(nil), names...)
	return m
}

func (m *Exec) EnvNames() []string {
	return append([]string(nil), m.env...)
}

// Refresh executes the configured shell command under ctx's deadline, sanitizes
// stdout, and returns a snapshot. Timeouts yield Stale; non-zero exits set Err
// but still populate Value so the renderer can show the last formatted output.
func (m *Exec) Refresh(ctx context.Context) status.ModuleSnapshot {
	return m.RefreshWithEnv(ctx, nil, "")
}

// RefreshWithEnv runs the command with env overlaid on the process environment and,
// when dir is a real directory, from that working directory (the interactive
// shell's cwd) so directory-sensitive tools (git, gh, mise) see the same context
// the user does. A missing or invalid dir falls back to ptyline's own cwd rather
// than failing the command.
func (m *Exec) RefreshWithEnv(ctx context.Context, env []string, dir string) status.ModuleSnapshot {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.timeout)
		defer cancel()
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", m.command)
	if len(env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), env)
	}
	if dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			cmd.Dir = dir
		}
	}
	cmd.Stdin = nil
	cmd.Stdout = io.Writer(&limitWriter{buf: &stdoutBuf, limit: execStdoutLimit})
	cmd.Stderr = io.Writer(&limitWriter{buf: &stderrBuf, limit: execStderrLimit})
	// On a timeout, ctx cancellation kills only /bin/sh; setProcessGroup puts the
	// child in its own group so the whole group can be killed (Unix), and WaitDelay
	// force-closes the pipes if a grandchild keeps them open, so Run cannot hang
	// past the deadline and leak this goroutine.
	setProcessGroup(cmd)
	cmd.WaitDelay = execWaitDelay

	runErr := cmd.Run()

	if ctx.Err() != nil {
		return status.ModuleSnapshot{
			ID:        m.ID(),
			Value:     status.Text(""),
			Stale:     true,
			Err:       ctx.Err(),
			UpdatedAt: time.Now(),
		}
	}

	exitCode := 0
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			exitCode = ee.ExitCode()
		}
	}

	stdout := sanitizeExecOutput(stdoutBuf.String())
	stderr := sanitizeExecOutput(stderrBuf.String())
	text := m.format
	text = strings.ReplaceAll(text, "{stdout}", stdout)
	text = strings.ReplaceAll(text, "{stderr}", stderr)
	text = strings.ReplaceAll(text, "{exit_code}", fmt.Sprintf("%d", exitCode))
	text = formatting.CollapseSeparators(text, m.separator)
	text = width.Truncate(text, m.maxWidth, "right")

	snap := status.ModuleSnapshot{
		ID:        m.ID(),
		Value:     status.Text(text),
		UpdatedAt: time.Now(),
	}
	if runErr != nil {
		snap.Err = runErr
	}
	return snap
}

// sanitizeExecOutput trims trailing whitespace and replaces control characters
// (including newlines) with spaces so command output is safe to embed in a
// single-line status bar.
func sanitizeExecOutput(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\n' || c == '\r':
			b.WriteByte(' ')
		case c == '\t':
			b.WriteByte(' ')
		case c < 0x20 || c == 0x7f:
			// strip other control characters
		default:
			b.WriteByte(c)
		}
	}
	return strings.TrimSpace(b.String())
}

// limitWriter is an io.Writer that stops accepting bytes after limit is reached.
type limitWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (w *limitWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil // silently discard
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return w.buf.Write(p)
}

func mergeEnv(base, overlay []string) []string {
	out := append([]string(nil), base...)
	for _, entry := range overlay {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		replaced := false
		prefix := key + "="
		for i, current := range out {
			if strings.HasPrefix(current, prefix) {
				out[i] = entry
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, entry)
		}
	}
	return out
}
