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

// TestShellEmitsNonceTaggedCWD sources each integration script with a known
// PTYLINE_NONCE, drives a cwd emit, and asserts the emitted cwd frame is
// "<nonce>:<path>". The Go side authenticates cwd with this nonce because it
// drives exec-module working directories and project .ptyline discovery, so a
// template that drops the prefix would silently disable that protection.
func TestShellEmitsNonceTaggedCWD(t *testing.T) {
	const nonce = "n0nce123"
	cases := []struct {
		shell string
		flags []string
		cmdFn func(scriptPath string) string
	}{
		{
			shell: "bash",
			flags: []string{"--noprofile", "--norc", "-c"},
			cmdFn: func(p string) string {
				return `export PTYLINE_NONCE=` + nonce + `; source ` + p + `; __ptyline_precmd`
			},
		},
		{
			shell: "zsh",
			flags: []string{"--no-rcs", "-c"},
			cmdFn: func(p string) string {
				return `export PTYLINE_NONCE=` + nonce + `; source ` + p + `; __ptyline_precmd`
			},
		},
		{
			shell: "fish",
			flags: []string{"--no-config", "-c"},
			cmdFn: func(p string) string {
				return `set -gx PTYLINE_NONCE ` + nonce + `; source ` + p + `; __ptyline_postexec`
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.shell, func(t *testing.T) {
			if _, err := exec.LookPath(tc.shell); err != nil {
				t.Skipf("%s not in PATH", tc.shell)
			}
			scriptPath, _ := filepath.Abs(writeScript(t, tc.shell))
			args := append(tc.flags, tc.cmdFn(scriptPath))
			var out, errBuf bytes.Buffer
			cmd := exec.Command(tc.shell, args...)
			cmd.Stdout = &out
			cmd.Stderr = &errBuf
			if err := cmd.Run(); err != nil {
				t.Logf("stderr: %s", errBuf.String())
				t.Fatalf("shell command failed: %v", err)
			}

			filter := proxy.NewAnsiFilter(reserved.Default())
			filter.Filter(out.Bytes())
			found := map[string]string{}
			for _, m := range filter.DrainMeta() {
				found[m.Key] = m.Value
			}

			cwd, ok := found[shellintegration.KeyCWD]
			if !ok {
				t.Fatalf("cwd not emitted; got keys: %v", keysOf(found))
			}
			want := nonce + ":"
			if !strings.HasPrefix(cwd, want) {
				t.Fatalf("cwd = %q, want %q prefix", cwd, want)
			}
			if strings.TrimPrefix(cwd, want) == "" {
				t.Fatalf("cwd = %q has an empty path after the nonce", cwd)
			}
		})
	}
}

// TestBashPromptCommandNotLeaked guards against the DEBUG-trap regression where
// a user's pre-existing PROMPT_COMMAND ran unguarded (as siblings after
// __ptyline_precmd), so its commands were emitted as if the user had typed them
// — and, worse, wedged __ptyline_running so the next real command was dropped.
// The guard must now span the whole PROMPT_COMMAND chain.
func TestBashPromptCommandNotLeaked(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not in PATH")
	}
	scriptPath, _ := filepath.Abs(writeScript(t, "bash"))
	// A user PROMPT_COMMAND plus a stub emit that logs; drive two prompt cycles
	// with a real command in between, exactly as an interactive shell would.
	script := `export PTYLINE_NONCE=n
PROMPT_COMMAND='builtin true LEAKED_PROMPT_CMD'
source ` + scriptPath + `
eval "$PROMPT_COMMAND"
builtin : REAL_USER_CMD
eval "$PROMPT_COMMAND"`
	var out, errBuf bytes.Buffer
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", script)
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Logf("stderr: %s", errBuf.String())
		t.Fatalf("shell command failed: %v", err)
	}

	filter := proxy.NewAnsiFilter(reserved.Default())
	filter.Filter(out.Bytes())
	for _, m := range filter.DrainMeta() {
		if m.Key == shellintegration.KeyCommand && strings.Contains(m.Value, "LEAKED_PROMPT_CMD") {
			t.Fatalf("user PROMPT_COMMAND leaked as a command frame: %q", m.Value)
		}
	}
}

func keysOf(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
