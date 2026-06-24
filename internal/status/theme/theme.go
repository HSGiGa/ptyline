// Package theme translates semantic tokens into terminal escape sequences.
// Modules and blocks request tokens (ok/warn/error/accent, agent.running, …)
// rather than writing raw ANSI, so presets, light/dark, and no-color modes work
// uniformly (arch.md §16).
package theme

// Token is a semantic color/style name resolved by the active theme.
type Token string

const (
	TokenOK     Token = "ok"
	TokenWarn   Token = "warn"
	TokenError  Token = "error"
	TokenMuted  Token = "muted"
	TokenAccent Token = "accent"
)

// Theme resolves tokens to escape sequences for the current terminal
// capabilities (truecolor vs 256 vs no-color).
type Theme struct {
	NoColor bool
	tokens  map[Token]string // token → SGR escape
}

// New builds a theme from a token→escape map.
func New(tokens map[Token]string) *Theme {
	return &Theme{tokens: tokens}
}

// Escape returns the SGR sequence for a token, or "" in no-color mode / when
// the token is unknown.
//
// TODO scaffold (plan 10): build the default palette and support color-scheme
// presets (gruvbox, catppuccin, nord, solarized) and style presets.
func (t *Theme) Escape(tok Token) string {
	if t.NoColor {
		return ""
	}
	return t.tokens[tok]
}
