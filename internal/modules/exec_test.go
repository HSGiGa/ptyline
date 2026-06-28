package modules

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecSuccess(t *testing.T) {
	m := NewExec("exec", "echo hello", time.Second, time.Second, "{stdout}", "", 0)
	snap := m.Refresh(context.Background())
	if snap.Value.Text != "hello" {
		t.Fatalf("value = %q, want %q", snap.Value.Text, "hello")
	}
	if snap.Stale {
		t.Fatalf("snap unexpectedly stale")
	}
	if snap.Err != nil {
		t.Fatalf("unexpected error: %v", snap.Err)
	}
}

func TestExecTimeout(t *testing.T) {
	m := NewExec("exec", "while :; do :; done", time.Second, 10*time.Millisecond, "{stdout}", "", 0)
	snap := m.Refresh(context.Background())
	if !snap.Stale {
		t.Fatalf("timeout snapshot not stale: %+v", snap)
	}
	if !errors.Is(snap.Err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", snap.Err)
	}
	if snap.Value.Text != "" {
		t.Fatalf("timeout value = %q, want empty", snap.Value.Text)
	}
}

func TestExecNonzeroExit(t *testing.T) {
	m := NewExec("exec", "exit 1", time.Second, time.Second, "{stdout}", "", 0)
	snap := m.Refresh(context.Background())
	if snap.Err == nil {
		t.Fatalf("non-zero exit should set Err")
	}
	if snap.Stale {
		t.Fatalf("non-zero exit should not be stale")
	}
}

func TestExecSanitizeControlChars(t *testing.T) {
	m := NewExec("exec", `printf 'a\x01b\x1bc'`, time.Second, time.Second, "{stdout}", "", 0)
	snap := m.Refresh(context.Background())
	if strings.ContainsAny(snap.Value.Text, "\x01\x1b") {
		t.Fatalf("control chars not stripped: %q", snap.Value.Text)
	}
}

func TestExecTruncatesLargeOutput(t *testing.T) {
	// Generate well over 4096 bytes of output.
	m := NewExec("exec", "i=0; while [ $i -lt 5000 ]; do printf a; i=$((i+1)); done", time.Second, 5*time.Second, "{stdout}", "", 0)
	snap := m.Refresh(context.Background())
	if len(snap.Value.Text) > execStdoutLimit {
		t.Fatalf("output not truncated: len=%d", len(snap.Value.Text))
	}
}

func TestExecFormatPlaceholders(t *testing.T) {
	m := NewExec("exec", "echo out", time.Second, time.Second, "code={exit_code} out={stdout}", "", 0)
	snap := m.Refresh(context.Background())
	if !strings.Contains(snap.Value.Text, "code=0") {
		t.Fatalf("exit_code placeholder not replaced: %q", snap.Value.Text)
	}
	if !strings.Contains(snap.Value.Text, "out=out") {
		t.Fatalf("stdout placeholder not replaced: %q", snap.Value.Text)
	}
}

func TestExecConditionalSeparators(t *testing.T) {
	m := NewExec("exec", "echo out", time.Second, time.Second, "{stdout} | {stderr} | {exit_code}", "•", 0)
	snap := m.Refresh(context.Background())
	if snap.Value.Text != "out • 0" {
		t.Fatalf("exec conditional separators = %q, want out • 0", snap.Value.Text)
	}
}

func TestExecMultilineCollapsed(t *testing.T) {
	m := NewExec("exec", "printf 'line1\\nline2\\nline3'", time.Second, time.Second, "{stdout}", "", 0)
	snap := m.Refresh(context.Background())
	if strings.Contains(snap.Value.Text, "\n") {
		t.Fatalf("newlines not collapsed: %q", snap.Value.Text)
	}
	if !strings.Contains(snap.Value.Text, "line1") || !strings.Contains(snap.Value.Text, "line2") {
		t.Fatalf("content lost during collapse: %q", snap.Value.Text)
	}
}

func TestExecDefaultFormat(t *testing.T) {
	m := NewExec("exec", "echo world", time.Second, time.Second, "", "", 0)
	if m.format != "{stdout}" {
		t.Fatalf("default format = %q, want {stdout}", m.format)
	}
	snap := m.Refresh(context.Background())
	if snap.Value.Text != "world" {
		t.Fatalf("value = %q, want world", snap.Value.Text)
	}
}

func TestExecMaxWidth(t *testing.T) {
	m := NewExec("exec", "printf 'abcdefghij'", time.Second, time.Second, "{stdout}", "", 5)
	snap := m.Refresh(context.Background())
	if len([]rune(snap.Value.Text)) > 5 {
		t.Fatalf("output not truncated to maxWidth=5: %q (len=%d)", snap.Value.Text, len([]rune(snap.Value.Text)))
	}
}

func TestExecDefaultMaxWidth(t *testing.T) {
	// maxWidth=0 applies the default cap of 60 cells.
	m := NewExec("exec", "echo hello", time.Second, time.Second, "{stdout}", "", 0)
	if m.maxWidth != defaultExecMaxWidth {
		t.Fatalf("default maxWidth = %d, want %d", m.maxWidth, defaultExecMaxWidth)
	}
}
