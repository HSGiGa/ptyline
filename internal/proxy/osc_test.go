package proxy

import (
	"strings"
	"testing"
)

func TestParseOSC777(t *testing.T) {
	cases := []struct {
		name, in, key, val string
		ok                 bool
	}{
		{"cwd", "cwd=/home/u", "cwd", "/home/u", true},
		{"exit_code", "exit_code=0", "exit_code", "0", true},
		{"command", "command=ls -la", "command", "ls -la", true},
		{"env", "env=staging", "env", "staging", true},
		{"not whitelisted", "hostname=x", "", "", false},
		{"control char", "cwd=a\x01b", "", "", false},
		{"no equals", "cwd", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			k, v, ok := parseOSC777(c.in)
			if ok != c.ok || k != c.key || v != c.val {
				t.Fatalf("parseOSC777(%q) = (%q,%q,%t), want (%q,%q,%t)",
					c.in, k, v, ok, c.key, c.val, c.ok)
			}
		})
	}
}

// Oversized payloads (> 8 KiB) are rejected (spec §9, §16).
func TestParseOSC777Oversize(t *testing.T) {
	big := "cwd=" + strings.Repeat("x", maxOSCPayload+1)
	if _, _, ok := parseOSC777(big); ok {
		t.Fatal("oversized OSC 777 payload must be rejected")
	}
}
