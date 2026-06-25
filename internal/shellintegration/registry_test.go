package shellintegration

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Supported() is derived from the embedded templates directory, so it matches the
// .sh files on disk exactly — proving the registry is data-driven (adding a shell
// is a template file, no Go edit).
func TestSupportedMatchesTemplateDir(t *testing.T) {
	entries, err := os.ReadDir(templateDir)
	if err != nil {
		t.Fatalf("read templates dir: %v", err)
	}
	var want []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".sh"); ok {
			want = append(want, name)
		}
	}
	sort.Strings(want)
	if got := Supported(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Supported() = %v, want %v", got, want)
	}
}

// Every supported shell yields a non-empty script that emits OSC 777 for the
// whitelisted keys, and nothing executes the metadata.
func TestScriptEmitsWhitelistedKeys(t *testing.T) {
	for _, shell := range Supported() {
		script, ok := Script(shell)
		if !ok || script == "" {
			t.Fatalf("Script(%q) missing", shell)
		}
		if !strings.Contains(script, `\e]777;`) {
			t.Errorf("%s: script does not emit OSC 777", shell)
		}
		for _, key := range Keys {
			if !strings.Contains(script, key) {
				t.Errorf("%s: script never emits key %q", shell, key)
			}
		}
	}
}

func TestScriptsUseActiveCommandStartClearProtocol(t *testing.T) {
	for _, shell := range Supported() {
		script, ok := Script(shell)
		if !ok {
			t.Fatalf("Script(%q) missing", shell)
		}
		start := strings.Index(script, `__ptyline_emit command "$__ptyline_cmd"`)
		clear := strings.Index(script, `__ptyline_emit command ""`)
		duration := strings.Index(script, `__ptyline_emit duration_ms`)
		if start < 0 {
			t.Errorf("%s: script does not emit active command on preexec", shell)
		}
		if clear < 0 {
			t.Errorf("%s: script does not clear active command", shell)
		}
		if duration < 0 {
			t.Errorf("%s: script does not emit duration", shell)
		}
		if duration >= 0 && clear >= 0 && clear < duration {
			t.Errorf("%s: script clears active command before duration", shell)
		}
	}
}

// Unknown shells and path-traversal attempts are rejected.
func TestScriptRejectsBadNames(t *testing.T) {
	for _, name := range []string{"", "nushell", "../osc", "bash.sh", filepath.Join("..", "osc")} {
		if _, ok := Script(name); ok {
			t.Errorf("Script(%q) accepted, want rejected", name)
		}
	}
}

func TestAllowedSetMatchesKeys(t *testing.T) {
	set := AllowedSet()
	if len(set) != len(Keys) {
		t.Fatalf("AllowedSet size %d, want %d", len(set), len(Keys))
	}
	for _, k := range Keys {
		if !set[k] {
			t.Errorf("AllowedSet missing %q", k)
		}
	}
}
