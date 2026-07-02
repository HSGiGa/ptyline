package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/runtimeenv"
	"github.com/hsgiga/ptyline/internal/status/theme"
)

// colorMode maps the detected terminal color level to a theme render mode.
func colorMode(level runtimeenv.ColorLevel) theme.Mode {
	switch level {
	case runtimeenv.ColorTrue:
		return theme.TrueColor
	case runtimeenv.Color256:
		return theme.Color256
	case runtimeenv.ColorBasic:
		return theme.Color16
	default:
		return theme.NoColor
	}
}

// resolveChild picks the command to run inside the PTY: explicit argv, else the
// configured shell, else $SHELL (spec §14).
func resolveChild(child []string, cfg config.Config, _ runtimeenv.Profile) []string {
	if len(child) > 0 {
		return child
	}
	if cfg.Shell != "" && cfg.Shell != "auto" {
		return []string{cfg.Shell}
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return []string{sh}
	}
	return []string{"/bin/sh"}
}

// moduleInterval returns the configured refresh interval or fallback.
func moduleInterval(cfg config.ModuleConfig, fallback time.Duration) time.Duration {
	if cfg.IntervalMS <= 0 {
		return fallback
	}
	return time.Duration(cfg.IntervalMS) * time.Millisecond
}

// moduleTimeout returns the configured per-refresh timeout or fallback.
func moduleTimeout(cfg config.ModuleConfig, fallback time.Duration) time.Duration {
	if cfg.TimeoutMS <= 0 {
		return fallback
	}
	return time.Duration(cfg.TimeoutMS) * time.Millisecond
}

// execEnvNames returns the deduped, sorted set of environment-variable patterns
// that exec modules want mirrored from the interactive shell. A pattern is either
// an exact name (GH_TOKEN) or a prefix with a single trailing '*' (GH_*); invalid
// entries are dropped so PTYLINE_EXEC_ENV_NAMES can never expand to the whole env.
func execEnvNames(cfg config.Config) []string {
	seen := map[string]bool{}
	var names []string
	for id, module := range cfg.Modules {
		if config.ModuleSource(id, module) != "exec" || module.Command == "" {
			continue
		}
		for _, name := range module.Env {
			if !isValidEnvPattern(name) || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// parseExecEnv decodes an exec_env frame of the form
// "<nonce>:<name>=<base64(value)>;<name>=<base64(value)>;…". It returns nil when
// the frame does not carry the expected nonce (spoofed or from another session)
// so callers leave existing state untouched; a valid frame always yields a
// non-nil map (possibly empty) representing the shell's complete current snapshot
// of the matched variables, which the caller applies wholesale so unset variables
// disappear.
func parseExecEnv(value, nonce string) map[string]string {
	if nonce == "" {
		return nil
	}
	got, entries, ok := strings.Cut(value, ":")
	if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(nonce)) != 1 {
		return nil
	}
	out := map[string]string{}
	for _, part := range strings.Split(entries, ";") {
		if part == "" {
			continue
		}
		name, encoded, ok := strings.Cut(part, "=")
		if !ok || !isValidEnvName(name) {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		out[name] = string(decoded)
	}
	return out
}

// stripNonce validates and removes the "<nonce>:" prefix that the shell
// integration prepends to authenticated single-value frames (cwd). It returns
// the payload and true only when the frame carries the expected nonce, so a
// forged OSC 777 (injected from a file or command output, which cannot know the
// per-session nonce) is rejected. An empty nonce never matches.
func stripNonce(value, nonce string) (string, bool) {
	if nonce == "" {
		return "", false
	}
	got, rest, ok := strings.Cut(value, ":")
	if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(nonce)) != 1 {
		return "", false
	}
	return rest, true
}

// isValidEnvName reports whether s is a POSIX-shell environment variable name
// (leading letter or underscore, then letters/digits/underscores).
func isValidEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'):
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// isValidEnvPattern reports whether s is an exact env name or a prefix followed by
// a single trailing '*'. A bare '*' or empty prefix is rejected.
func isValidEnvPattern(s string) bool {
	if prefix, ok := strings.CutSuffix(s, "*"); ok {
		return isValidEnvName(prefix)
	}
	return isValidEnvName(s)
}

// changedEnvNames returns the variable names whose value differs between two
// snapshots, including names added in next or removed from old.
func changedEnvNames(old, next map[string]string) []string {
	var changed []string
	for name, value := range next {
		if prev, ok := old[name]; !ok || prev != value {
			changed = append(changed, name)
		}
	}
	for name := range old {
		if _, ok := next[name]; !ok {
			changed = append(changed, name)
		}
	}
	return changed
}

// envNameMatches reports whether a concrete variable name matches a pattern
// (exact name, or prefix when the pattern ends in '*').
func envNameMatches(name, pattern string) bool {
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(name, prefix)
	}
	return name == pattern
}

// newNonce returns a random hex token used to authenticate exec_env frames.
// An empty result (RNG failure) disables the channel: the shell emits nothing and
// parseExecEnv rejects every frame.
func newNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
