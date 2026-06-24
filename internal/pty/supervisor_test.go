//go:build unix

package pty

import (
	"testing"

	"github.com/hsgiga/ptyline/internal/reserved"
)

// childSize must always reserve the bar rows: 30 real rows → 29 child rows, never
// a hardcoded rows-1 and never < 1 (spec §8.2).
func TestChildSizeReservesRows(t *testing.T) {
	s := New([]string{"/bin/sh"}, reserved.Default())
	if got := s.childSize(Size{Cols: 80, Rows: 30}); got != (Size{Cols: 80, Rows: 29}) {
		t.Fatalf("childSize(80x30) = %+v, want 80x29", got)
	}
	// Degenerate: a 1-row terminal still gives the child at least one row.
	if got := s.childSize(Size{Cols: 80, Rows: 1}); got.Rows != 1 {
		t.Fatalf("childSize(80x1).Rows = %d, want 1", got.Rows)
	}
}

// The wrapper exits with the child's exit code (spec §8.2).
func TestWaitPropagatesExitCode(t *testing.T) {
	s := New([]string{"/bin/sh", "-c", "exit 7"}, reserved.Default())
	if err := s.Start(Size{Cols: 80, Rows: 24}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	code, err := s.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
}
