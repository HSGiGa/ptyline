// External test package — imports proxy without cycle (same as roundtrip_test.go).
package shellintegration_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hsgiga/ptyline/internal/proxy"
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/shellintegration"
)

// writeScript writes the embedded integration script for shell to a temp file
// and returns its path.
func writeScript(t *testing.T, shell string) string {
	t.Helper()
	script, ok := shellintegration.Script(shell)
	if !ok {
		t.Fatalf("no integration script for shell %q", shell)
	}
	f, err := os.CreateTemp(t.TempDir(), "ptyline-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(script); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

// TestShellEmitsDurationAsInteger sources each integration script in its shell,
// drives the pre/post-exec lifecycle manually, and asserts that duration_ms in
// the emitted OSC 777 stream is a valid non-negative integer.
//
// Regression test for H1: GNU-only `date +%s%3N` broke duration_ms on macOS
// for bash and zsh.
func TestShellEmitsDurationAsInteger(t *testing.T) {
	type testCase struct {
		shell string
		flags []string
		// cmdFn builds the shell command string from the integration script path.
		cmdFn func(scriptPath string) string
	}

	cases := []testCase{
		{
			shell: "bash",
			flags: []string{"--noprofile", "--norc", "-c"},
			// Set __ptyline_running to simulate that preexec fired, then call
			// precmd which reads __ptyline_start and emits duration_ms.
			cmdFn: func(p string) string {
				return `source ` + p + `; __ptyline_running=1; __ptyline_start=$(__ptyline_now_ms); __ptyline_precmd`
			},
		},
		{
			shell: "zsh",
			flags: []string{"--no-rcs", "-c"},
			// precmd checks __ptyline_start (non-empty → emit duration_ms).
			cmdFn: func(p string) string {
				return `source ` + p + `; __ptyline_start=$(__ptyline_now_ms); __ptyline_precmd`
			},
		},
		{
			shell: "fish",
			flags: []string{"--no-config", "-c"},
			// Call __ptyline_postexec directly (also registered as fish_postexec
			// event handler). Set __ptyline_start so it emits duration_ms.
			cmdFn: func(p string) string {
				return `source ` + p + `; set -g __ptyline_start (__ptyline_ms_now); __ptyline_postexec`
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.shell, func(t *testing.T) {
			if _, err := exec.LookPath(tc.shell); err != nil {
				t.Skipf("%s not in PATH", tc.shell)
			}

			scriptPath := writeScript(t, tc.shell)
			scriptPath, _ = filepath.Abs(scriptPath)

			args := append(tc.flags, tc.cmdFn(scriptPath))
			var out, errBuf bytes.Buffer
			cmd := exec.Command(tc.shell, args...)
			cmd.Stdout = &out
			cmd.Stderr = &errBuf

			if err := cmd.Run(); err != nil {
				t.Logf("stderr: %s", errBuf.String())
				t.Fatalf("shell command failed: %v", err)
			}
			if errBuf.Len() > 0 {
				t.Logf("stderr (non-fatal): %s", errBuf.String())
			}

			output := out.Bytes()
			t.Logf("raw output: %q", output)

			// Feed through the real ANSI filter — all OSC 777 frames must be consumed.
			filter := proxy.NewAnsiFilter(reserved.Default())
			filtered := filter.Filter(output)
			if bytes.Contains(filtered, []byte("777")) {
				t.Errorf("OSC 777 frame leaked to terminal output: %q", filtered)
			}

			found := map[string]string{}
			for _, m := range filter.DrainMeta() {
				found[m.Key] = m.Value
			}

			durStr, ok := found[shellintegration.KeyDurationMS]
			if !ok {
				t.Fatalf("duration_ms not emitted; got keys: %v", keysOf(found))
			}
			dur, err := strconv.ParseInt(strings.TrimSpace(durStr), 10, 64)
			if err != nil || dur < 0 {
				t.Fatalf("duration_ms = %q, want non-negative integer: %v", durStr, err)
			}
			t.Logf("duration_ms = %d ms", dur)
		})
	}
}

func keysOf(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
