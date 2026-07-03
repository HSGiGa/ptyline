//go:build unix

package app

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
)

func encodeHandoff(t *testing.T, hs handoffState) string {
	t.Helper()
	data, err := json.Marshal(hs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func TestParseHandoffAbsent(t *testing.T) {
	t.Setenv(handoffEnvKey, "") // empty is treated as absent
	state, present, err := parseHandoff()
	if err != nil {
		t.Fatalf("want nil err, got %v", err)
	}
	if present {
		t.Fatal("want present=false")
	}
	if state != nil {
		t.Fatal("want nil state")
	}
}

func TestParseHandoffRoundTrip(t *testing.T) {
	want := handoffState{
		Version:   handoffVersion,
		PtyFD:     5,
		ChildPID:  12345,
		Nonce:     "abc123",
		ChildArgv: []string{"/bin/zsh"},
	}
	t.Setenv(handoffEnvKey, encodeHandoff(t, want))

	state, present, err := parseHandoff()
	if err != nil {
		t.Fatalf("parseHandoff: %v", err)
	}
	if !present {
		t.Fatal("want present=true")
	}
	if state == nil {
		t.Fatal("want non-nil state")
	}
	if state.PtyFD != want.PtyFD || state.ChildPID != want.ChildPID ||
		state.Nonce != want.Nonce || len(state.ChildArgv) != 1 || state.ChildArgv[0] != want.ChildArgv[0] {
		t.Fatalf("state mismatch: got %+v, want %+v", state, want)
	}
	// Env must be cleared after parse.
	if os.Getenv(handoffEnvKey) != "" {
		t.Fatal("PTYLINE_HANDOFF not cleared after parseHandoff")
	}
}

func TestParseHandoffGarbage(t *testing.T) {
	t.Setenv(handoffEnvKey, "not-valid-base64!!!")

	state, present, err := parseHandoff()
	if err == nil {
		t.Fatal("want error for garbage handoff")
	}
	if !present {
		t.Fatal("want present=true even on error")
	}
	if state != nil {
		t.Fatal("want nil state on error")
	}
	// Env must be cleared even on error.
	if os.Getenv(handoffEnvKey) != "" {
		t.Fatal("PTYLINE_HANDOFF not cleared after failed parseHandoff")
	}
}

func TestParseHandoffWrongVersion(t *testing.T) {
	hs := handoffState{Version: handoffVersion + 1, PtyFD: 3, ChildPID: 99}
	t.Setenv(handoffEnvKey, encodeHandoff(t, hs))

	state, present, err := parseHandoff()
	if err == nil {
		t.Fatal("want error for wrong version")
	}
	if !present {
		t.Fatal("want present=true")
	}
	if state != nil {
		t.Fatal("want nil state")
	}
}
