package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	assets "github.com/hsgiga/ptyline/config"
	"github.com/hsgiga/ptyline/internal/format"
)

// Load reads, migrates, and parses the config. The flow is:
//
//	read file → migrate_to_latest → parse → infer active modules → merge over Default()
//
// If path is empty, DefaultPath() is used; a missing file yields Default()
// without error (spec §13).
//
// The second return value is the set of module ids the file explicitly disabled via
// enabled = false; pass it to ApplyOverlays so a later overlay's inferred activation
// can't silently resurrect a root-config disable.
func Load(path string) (Config, map[string]bool, error) {
	path = ResolvePath(path)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil, nil
		}
		return Config{}, nil, err
	}
	migrated, err := migrateToLatest(raw)
	if err != nil {
		return Config{}, nil, err
	}
	cfg, explicitlyDisabled, err := parse(migrated)
	if err != nil {
		return Config{}, nil, fmt.Errorf("%s: %w", path, err)
	}
	inferActiveModules(&cfg, explicitlyDisabled)
	if err := Validate(&cfg); err != nil {
		return Config{}, nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, explicitlyDisabled, nil
}

// ResolvePath returns the effective config path for a user-supplied path.
func ResolvePath(path string) string {
	if path != "" {
		return path
	}
	return DefaultPath()
}

// parse decodes already-migrated TOML bytes into a Config layered over Default().
// The returned map holds the ids of modules the file explicitly disabled via
// enabled = false (see Load).
func parse(raw []byte) (Config, map[string]bool, error) {
	cfg := Default()
	metadata, err := toml.Decode(string(raw), &cfg)
	if err != nil {
		return Config{}, nil, err
	}
	if !metadata.IsDefined("config_version") {
		return Config{}, nil, fmt.Errorf("config_version is required")
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return Config{}, nil, fmt.Errorf("unknown key %q", undecoded[0])
	}
	// A user who specifies `format` but no `[[bar.row]]` overrides the multi-line
	// default; otherwise the default rows would shadow their intent.
	if !metadata.IsDefined("bar", "row") && metadata.IsDefined("bar", "format") {
		cfg.Bar.Rows = nil
	}
	explicitlyDisabled := map[string]bool{}
	for id, mod := range cfg.Modules {
		if metadata.IsDefined("module", id, "enabled") && !mod.Enabled {
			explicitlyDisabled[id] = true
		}
	}
	return cfg, explicitlyDisabled, nil
}

// Validate enforces the MVP configuration contract (spec §13.1). Violations are
// startup errors that must name the offending key (the caller adds the file path):
//
//   - config_version is required and must equal CurrentVersion;
//   - unknown top-level/module keys, invalid enums, and invalid width expressions
//     are errors (not silently defaulted);
//   - bar must define format or at least one [[bar.row]].
func Validate(cfg *Config) error {
	if cfg.Version != CurrentVersion {
		return fmt.Errorf("config_version must be %d", CurrentVersion)
	}
	if cfg.Bar.Format == "" && len(cfg.Bar.Rows) == 0 {
		return fmt.Errorf("bar must define format or at least one [[bar.row]]")
	}
	if !oneOf(cfg.Bar.Justify, "left", "center", "right", "absolute_center") {
		return fmt.Errorf("bar.justify has invalid value %q", cfg.Bar.Justify)
	}
	if cfg.Bar.MinBlockWidth < 0 {
		return fmt.Errorf("bar.min_block_width must be >= 0")
	}
	if !oneOf(cfg.Icons.Preset, "nerd-font", "emoji", "ascii") {
		return fmt.Errorf("icons.preset has invalid value %q", cfg.Icons.Preset)
	}
	if !oneOf(cfg.Icons.EmojiWidth, "auto", "1", "2") {
		return fmt.Errorf("icons.emoji_width has invalid value %q", cfg.Icons.EmojiWidth)
	}
	for id, module := range cfg.Modules {
		source := ModuleSource(id, module)
		if module.Source != "" && !oneOf(module.Source, "time", "exec", "template") {
			return fmt.Errorf("module.%s.source has invalid value %q", id, module.Source)
		}
		if source == "template" && module.Format == "" {
			return fmt.Errorf("module.%s.format is required for source \"template\"", id)
		}
		if module.Provider != "" && !oneOf(module.Provider, "command", "exec") {
			return fmt.Errorf("module.%s.provider has invalid value %q", id, module.Provider)
		}
		if source == "exec" && module.Command == "" &&
			(module.Enabled || module.Source != "" || module.Provider != "" || len(module.RefreshOnCommand) > 0) {
			return fmt.Errorf("module.%s.command is required for source %q", id, source)
		}
		if module.Animation != "" && !oneOf(string(module.Animation), "none", "default", "glint", "pulse", "blink") {
			return fmt.Errorf("module.%s.animation has invalid value %q", id, module.Animation)
		}
		if module.AnimationIntervalMS < 0 {
			return fmt.Errorf("module.%s.animation_interval_ms must be >= 0", id)
		}
		if module.Icon != "" && !oneOf(module.Icon, "left", "right") {
			return fmt.Errorf("module.%s.icon has invalid value %q", id, module.Icon)
		}
		if module.MaxWidth < 0 {
			return fmt.Errorf("module.%s.max_width must be >= 0", id)
		}
		if module.ActiveMinDurationMS < 0 {
			return fmt.Errorf("module.%s.active_min_duration_ms must be >= 0", id)
		}
		if module.DoneMinDurationMS < 0 {
			return fmt.Errorf("module.%s.done_min_duration_ms must be >= 0", id)
		}
		if module.DoneSuccessTTLMS < 0 {
			return fmt.Errorf("module.%s.done_success_ttl_ms must be >= 0", id)
		}
		if module.DoneFailureTTLMS < 0 {
			return fmt.Errorf("module.%s.done_failure_ttl_ms must be >= 0", id)
		}
		// Exec modules mirror matching shell variables, so they may use a trailing
		// '*' prefix pattern (GH_*); the env display module needs exact names.
		allowEnvPattern := ModuleSource(id, module) == "exec"
		for _, name := range module.Env {
			if !validEnvNameOrPattern(name, allowEnvPattern) {
				return fmt.Errorf("module.%s.env has invalid value %q", id, name)
			}
		}
		for _, pattern := range module.RefreshOnCommand {
			if strings.Join(strings.Fields(pattern), " ") == "" {
				return fmt.Errorf("module.%s.refresh_on_command has an empty pattern", id)
			}
		}
	}
	// Exec and git modules with sub-100ms intervals would saturate the subprocess pool.
	const minExecIntervalMS = 100
	for id, module := range cfg.Modules {
		src := ModuleSource(id, module)
		if (src == "exec" || id == "git") && module.IntervalMS > 0 && module.IntervalMS < minExecIntervalMS {
			return fmt.Errorf("module.%s.interval_ms must be >= %d", id, minExecIntervalMS)
		}
	}
	// Cross-module check: template modules must not reference other templates or themselves.
	templateIDs := map[string]bool{}
	for id, module := range cfg.Modules {
		if ModuleSource(id, module) == "template" {
			templateIDs[id] = true
		}
	}
	for id, module := range cfg.Modules {
		if !templateIDs[id] {
			continue
		}
		for _, b := range format.ParseFormat(module.Format) {
			if b.IsLiteral() || b.IsSeparator() {
				continue
			}
			if b.ModuleID == id {
				return fmt.Errorf("module.%s: template cannot reference itself", id)
			}
			if templateIDs[b.ModuleID] {
				return fmt.Errorf("module.%s: template cannot reference another template module %q", id, b.ModuleID)
			}
		}
	}
	return nil
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

var builtinModuleIDs = map[string]bool{
	"time": true, "date": true, "hostname": true, "user": true, "runtime": true, "shell": true,
	"env": true, "cwd": true, "ssh": true, "command": true,
	"load": true, "cpu": true, "memory": true, "battery": true, "disk": true,
	// git and all sub-module placeholders
	"git": true, "git.branch": true,
	"git.staged": true, "git.modified": true, "git.untracked": true, "git.conflict": true,
	"git.ahead": true, "git.behind": true,
	"git.state": true, "git.dirty": true,
}

// IsBuiltinModuleID reports whether id is provided by ptyline itself.
func IsBuiltinModuleID(id string) bool {
	return builtinModuleIDs[id]
}

// ModuleSource resolves the effective source for a module. Unknown module IDs
// default to exec unless an explicit source/provider is configured.
func ModuleSource(id string, module ModuleConfig) string {
	if module.Source != "" {
		return module.Source
	}
	if module.Provider == "command" {
		return "exec"
	}
	if module.Provider != "" {
		return module.Provider
	}
	if !IsBuiltinModuleID(id) {
		return "exec"
	}
	return ""
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validEnvName(name string) bool {
	return envName.MatchString(name)
}

// validEnvNameOrPattern accepts an exact env name and, when allowPattern is set,
// a non-empty prefix followed by a single trailing '*' (GH_*).
func validEnvNameOrPattern(name string, allowPattern bool) bool {
	if allowPattern {
		if prefix, ok := strings.CutSuffix(name, "*"); ok {
			return validEnvName(prefix)
		}
	}
	return validEnvName(name)
}

// DefaultPath returns $XDG_CONFIG_HOME/ptyline/config.toml, falling back to
// ~/.config/ptyline/config.toml (spec §13).
func DefaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ptyline", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "ptyline", "config.toml")
	}
	return filepath.Join(home, ".config", "ptyline", "config.toml")
}

// EnsureUserConfig writes the sample config.toml to the default config path when
// none exists yet, giving a fresh install a file to edit. It never overwrites an
// existing config. Writing is best-effort and callers should treat an error as
// non-fatal: Load falls back to built-in defaults when no file is present, and
// themes/styles resolve from the binary regardless.
func EnsureUserConfig() (created bool, path string, err error) {
	path = DefaultPath()
	switch _, statErr := os.Stat(path); {
	case statErr == nil:
		return false, path, nil
	case !os.IsNotExist(statErr):
		return false, path, statErr
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, path, err
	}
	if err := os.WriteFile(path, assets.SampleConfig(), 0o644); err != nil {
		return false, path, err
	}
	return true, path, nil
}

// FindProjectConfig returns the closest .ptyline file at or above dir. Project
// configuration is deliberately opt-in: callers decide which fields to apply.
func FindProjectConfig(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(dir, ".ptyline")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// ResolveOverlayPath resolves a --ptyline argument to a file path. Short names
// (no path separator, no file extension) are resolved relative to the config
// directory; everything else is returned as-is.
func ResolveOverlayPath(name string) string {
	if name == "" {
		return ""
	}
	if strings.ContainsRune(name, filepath.Separator) || strings.Contains(name, ".") {
		return name
	}
	return filepath.Join(filepath.Dir(DefaultPath()), name+".ptyline")
}

// parseOverlay decodes TOML bytes into a zero-value Config (not Default()),
// returning the metadata needed to detect which keys were explicitly set.
func parseOverlay(raw []byte) (Config, toml.MetaData, error) {
	var cfg Config
	meta, err := toml.Decode(string(raw), &cfg)
	if err != nil {
		return Config{}, toml.MetaData{}, err
	}
	if !meta.IsDefined("config_version") {
		return Config{}, toml.MetaData{}, fmt.Errorf("config_version is required")
	}
	if cfg.Version != CurrentVersion {
		return Config{}, toml.MetaData{}, fmt.Errorf("config_version must be %d", CurrentVersion)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return Config{}, toml.MetaData{}, fmt.Errorf("unknown key %q", undecoded[0])
	}
	return cfg, meta, nil
}

// ValidateOverlayScope rejects fields that overlays are not allowed to set
// (spec: overlays must not change command execution or process-level settings).
func ValidateOverlayScope(overlay Config, meta toml.MetaData) error {
	if meta.IsDefined("shell") {
		return fmt.Errorf("overlay must not set shell")
	}
	if meta.IsDefined("refresh_interval_ms") {
		return fmt.Errorf("overlay must not set refresh_interval_ms")
	}
	for id, mod := range overlay.Modules {
		if meta.IsDefined("module", id, "command") {
			return fmt.Errorf("overlay must not set module.%s.command", id)
		}
		if meta.IsDefined("module", id, "timeout_ms") {
			return fmt.Errorf("overlay must not set module.%s.timeout_ms", id)
		}
		if meta.IsDefined("module", id, "refresh_on_command") {
			return fmt.Errorf("overlay must not set module.%s.refresh_on_command", id)
		}
		if meta.IsDefined("module", id, "env") && !IsBuiltinModuleID(id) {
			// Exec module env controls which shell variables are passed to subprocess
			// commands; allowing overlays to set it would let a project .ptyline
			// exfiltrate credentials (e.g. env = ["AWS_SECRET_ACCESS_KEY"]).
			return fmt.Errorf("overlay must not set module.%s.env", id)
		}
		if meta.IsDefined("module", id, "source") {
			return fmt.Errorf("overlay must not set module.%s.source", id)
		}
		if meta.IsDefined("module", id, "provider") && mod.Provider == "command" {
			return fmt.Errorf("overlay must not set module.%s.provider = \"command\"", id)
		}
	}
	return nil
}

// MergeOverlay applies an overlay on top of base. Map sections (modules, styles,
// theme palette maps) merge by key; slice fields (env, bar.row) replace entirely
// when the overlay defines them; scalar fields use the overlay value when non-zero;
// booleans use IsDefined to detect explicit sets.
func MergeOverlay(base, overlay Config, meta toml.MetaData) Config {
	result := base

	// Bar layout
	if overlay.Bar.Format != "" {
		result.Bar.Format = overlay.Bar.Format
	}
	if meta.IsDefined("bar", "row") {
		result.Bar.Rows = overlay.Bar.Rows
	}
	if overlay.Bar.Separator != "" {
		result.Bar.Separator = overlay.Bar.Separator
	}
	if overlay.Bar.Padding != 0 {
		result.Bar.Padding = overlay.Bar.Padding
	}
	if overlay.Bar.Justify != "" {
		result.Bar.Justify = overlay.Bar.Justify
	}
	if meta.IsDefined("bar", "min_block_width") {
		result.Bar.MinBlockWidth = overlay.Bar.MinBlockWidth
	}

	// Theme scalar fields
	if overlay.Theme.ColorScheme != "" {
		result.Theme.ColorScheme = overlay.Theme.ColorScheme
	}
	if overlay.Theme.Style != "" {
		result.Theme.Style = overlay.Theme.Style
	}
	if overlay.Theme.Icons != "" {
		result.Theme.Icons = overlay.Theme.Icons
	}
	if overlay.Theme.Fallback != "" {
		result.Theme.Fallback = overlay.Theme.Fallback
	}
	// Theme maps merge by key
	if result.Theme.Palette == nil {
		result.Theme.Palette = map[string]string{}
	}
	for k, v := range overlay.Theme.Palette {
		result.Theme.Palette[k] = v
	}
	if result.Theme.Status == nil {
		result.Theme.Status = map[string]string{}
	}
	for k, v := range overlay.Theme.Status {
		result.Theme.Status[k] = v
	}

	// Icons
	if meta.IsDefined("icons", "enabled") {
		result.Icons.Enabled = overlay.Icons.Enabled
	}
	if overlay.Icons.Preset != "" {
		result.Icons.Preset = overlay.Icons.Preset
	}
	if overlay.Icons.EmojiWidth != "" {
		result.Icons.EmojiWidth = overlay.Icons.EmojiWidth
	}
	if meta.IsDefined("icons", "fallback") {
		result.Icons.Fallback = overlay.Icons.Fallback
	}

	// Modules — merge by key
	for id, ovMod := range overlay.Modules {
		base := ModuleConfig{}
		if existing, ok := result.Modules[id]; ok {
			base = existing
		}
		merged := mergeModuleConfig(base, ovMod, meta, id)
		if result.Modules == nil {
			result.Modules = map[string]ModuleConfig{}
		}
		result.Modules[id] = merged
	}

	// Styles — merge by key at sub-field level
	for id, ovStyle := range overlay.Styles {
		base := StyleConfig{}
		if existing, ok := result.Styles[id]; ok {
			base = existing
		}
		if result.Styles == nil {
			result.Styles = map[string]StyleConfig{}
		}
		result.Styles[id] = mergeStyleConfig(base, ovStyle)
	}

	return result
}

func mergeModuleConfig(base, overlay ModuleConfig, meta toml.MetaData, id string) ModuleConfig {
	if meta.IsDefined("module", id, "enabled") {
		base.Enabled = overlay.Enabled
	}
	if meta.IsDefined("module", id, "collapse_whitespace") {
		base.CollapseWhitespace = overlay.CollapseWhitespace
	}
	if meta.IsDefined("module", id, "hide_when_empty") {
		base.HideWhenEmpty = overlay.HideWhenEmpty
	}
	if overlay.Format != "" {
		base.Format = overlay.Format
	}
	if meta.IsDefined("module", id, "separator") {
		base.Separator = overlay.Separator
	}
	if overlay.Source != "" {
		base.Source = overlay.Source
	}
	if overlay.Mode != "" {
		base.Mode = overlay.Mode
	}
	if overlay.Provider != "" {
		base.Provider = overlay.Provider
	}
	if meta.IsDefined("module", id, "env") {
		base.Env = overlay.Env
	}
	if overlay.IntervalMS != 0 {
		base.IntervalMS = overlay.IntervalMS
	}
	if overlay.MaxWidth != 0 {
		base.MaxWidth = overlay.MaxWidth
	}
	if overlay.ActiveMinDurationMS != 0 {
		base.ActiveMinDurationMS = overlay.ActiveMinDurationMS
	}
	if meta.IsDefined("module", id, "animation") {
		base.Animation = overlay.Animation
	}
	if overlay.AnimationIntervalMS != 0 {
		base.AnimationIntervalMS = overlay.AnimationIntervalMS
	}
	if meta.IsDefined("module", id, "icon") {
		base.Icon = overlay.Icon
	}
	if meta.IsDefined("module", id, "icon_glyph") {
		base.IconGlyph = overlay.IconGlyph
	}
	if meta.IsDefined("module", id, "icon_fallback") {
		base.IconFallback = overlay.IconFallback
	}
	return base
}

func mergeStyleConfig(base, overlay StyleConfig) StyleConfig {
	if overlay.FG != "" {
		base.FG = overlay.FG
	}
	if overlay.BG != "" {
		base.BG = overlay.BG
	}
	if overlay.Bold {
		base.Bold = true
	}
	if overlay.Dim {
		base.Dim = true
	}
	if overlay.Italic {
		base.Italic = true
	}
	if overlay.Underline {
		base.Underline = true
	}
	if overlay.Animation != "" {
		base.Animation = overlay.Animation
	}
	if overlay.Shape != "" {
		base.Shape = overlay.Shape
	}
	if overlay.LeftCap != "" {
		base.LeftCap = overlay.LeftCap
	}
	if overlay.RightCap != "" {
		base.RightCap = overlay.RightCap
	}
	if overlay.PaddingLeft != 0 {
		base.PaddingLeft = overlay.PaddingLeft
	}
	if overlay.PaddingRight != 0 {
		base.PaddingRight = overlay.PaddingRight
	}
	return base
}

// inferActiveModules enables any module referenced in the bar layout (by a
// {name}/{name:spec} placeholder) unless it was explicitly disabled via
// enabled=false (tracked by the caller, e.g. Load or ApplyOverlays).
func inferActiveModules(cfg *Config, explicitlyDisabled map[string]bool) {
	activate := func(id string) {
		if explicitlyDisabled[id] {
			return
		}
		if cfg.Modules == nil {
			cfg.Modules = map[string]ModuleConfig{}
		}
		m := cfg.Modules[id]
		m.Enabled = true
		cfg.Modules[id] = m
	}
	scan := func(formatStr string) {
		for _, block := range format.ParseFormat(formatStr) {
			if block.IsLiteral() || block.IsSeparator() {
				continue
			}
			activate(block.ModuleID)
		}
	}
	scan(cfg.Bar.Format)
	for _, row := range cfg.Bar.Rows {
		scan(row.Format)
	}
}

// ApplyOverlays merges each overlay path (in order, highest precedence last)
// on top of base, then infers active modules. Empty paths are skipped.
// rootExplicitlyDisabled carries forward the ids the root config file
// explicitly disabled (via Load), so overlay-triggered inference can't
// silently re-enable them.
func ApplyOverlays(base Config, rootExplicitlyDisabled map[string]bool, paths ...string) (Config, error) {
	result := base
	explicitlyDisabled := map[string]bool{}
	for id := range rootExplicitlyDisabled {
		explicitlyDisabled[id] = true
	}

	for _, path := range paths {
		if path == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("overlay %s: %w", path, err)
		}
		overlay, meta, err := parseOverlay(raw)
		if err != nil {
			return Config{}, fmt.Errorf("overlay %s: %w", path, err)
		}
		if err := ValidateOverlayScope(overlay, meta); err != nil {
			return Config{}, fmt.Errorf("overlay %s: %w", path, err)
		}
		for id, mod := range overlay.Modules {
			if meta.IsDefined("module", id, "enabled") {
				if mod.Enabled {
					delete(explicitlyDisabled, id)
				} else {
					explicitlyDisabled[id] = true
				}
			}
		}
		result = MergeOverlay(result, overlay, meta)
	}

	inferActiveModules(&result, explicitlyDisabled)
	if err := Validate(&result); err != nil {
		return Config{}, fmt.Errorf("merged config invalid: %w", err)
	}
	return result, nil
}
