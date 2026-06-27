package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Load reads, migrates, and parses the config. The flow is:
//
//	read file → migrate_to_latest → parse → merge over Default()
//
// If path is empty, DefaultPath() is used; a missing file yields Default()
// without error (spec §13).
func Load(path string) (Config, error) {
	path = ResolvePath(path)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, err
	}
	migrated, err := migrateToLatest(raw)
	if err != nil {
		return Config{}, err
	}
	cfg, err := parse(migrated)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := Validate(&cfg); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// ResolvePath returns the effective config path for a user-supplied path.
func ResolvePath(path string) string {
	if path != "" {
		return path
	}
	return DefaultPath()
}

// parse decodes already-migrated TOML bytes into a Config layered over Default().
func parse(raw []byte) (Config, error) {
	cfg := Default()
	metadata, err := toml.Decode(string(raw), &cfg)
	if err != nil {
		return Config{}, err
	}
	if !metadata.IsDefined("config_version") {
		return Config{}, fmt.Errorf("config_version is required")
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("unknown key %q", undecoded[0])
	}
	// A user who specifies a single-line `format` (or `[[bar.block]]`) but no
	// `[[bar.row]]` overrides the multi-line default; otherwise the default rows
	// would shadow their intent (defaults fill unset fields).
	if !metadata.IsDefined("bar", "row") &&
		(metadata.IsDefined("bar", "format") || metadata.IsDefined("bar", "block")) {
		cfg.Bar.Rows = nil
	}
	return cfg, nil
}

// Validate enforces the MVP configuration contract (spec §13.1). Violations are
// startup errors that must name the offending key (the caller adds the file path):
//
//   - config_version is required and must equal CurrentVersion;
//   - unknown top-level/module keys, invalid enums, and invalid width expressions
//     are errors (not silently defaulted);
//   - `bar.format` and `[[bar.block]]` are mutually exclusive;
//   - the reserved height is the number of `[[bar.row]]` entries (or 1 for the
//     single-line `bar.format` fallback) and must be >= 1;
//   - module IDs are unique and a block references an enabled module by ID;
//   - custom-command modules must carry a timeout (spec §16, §17).
func Validate(cfg *Config) error {
	if cfg.Version != CurrentVersion {
		return fmt.Errorf("config_version must be %d", CurrentVersion)
	}
	// Multi-line: the reserved height follows the [[bar.row]] count when present.
	if len(cfg.Bar.Rows) > 0 {
		cfg.Bar.Height = uint16(len(cfg.Bar.Rows))
	}
	if cfg.Bar.Height < 1 {
		return fmt.Errorf("bar.height must be >= 1")
	}
	if cfg.Bar.Format != "" && len(cfg.Bar.Blocks) > 0 {
		return fmt.Errorf("bar.format and bar.block are mutually exclusive")
	}
	if !oneOf(cfg.Bar.Mode, "single-line", "agent-panel") {
		return fmt.Errorf("bar.mode has invalid value %q", cfg.Bar.Mode)
	}
	if !oneOf(cfg.Icons.Preset, "nerd-font", "emoji", "ascii") {
		return fmt.Errorf("icons.preset has invalid value %q", cfg.Icons.Preset)
	}
	if !oneOf(cfg.Icons.EmojiWidth, "auto", "1", "2") {
		return fmt.Errorf("icons.emoji_width has invalid value %q", cfg.Icons.EmojiWidth)
	}
	for index, block := range cfg.Bar.Blocks {
		prefix := fmt.Sprintf("bar.block[%d]", index)
		if !oneOf(block.Anchor, "left", "center", "right") {
			return fmt.Errorf("%s.anchor has invalid value %q", prefix, block.Anchor)
		}
		if !oneOf(block.Align, "left", "center", "right") {
			return fmt.Errorf("%s.align has invalid value %q", prefix, block.Align)
		}
		if !oneOf(block.Truncate, "left", "right", "middle", "none") {
			return fmt.Errorf("%s.truncate has invalid value %q", prefix, block.Truncate)
		}
		for name, width := range map[string]string{"width": block.Width, "min_width": block.MinWidth, "max_width": block.MaxWidth} {
			if width != "" && !validWidth(width) {
				return fmt.Errorf("%s.%s has invalid value %q", prefix, name, width)
			}
		}
		module, ok := cfg.Modules[block.Module]
		if !ok || !module.Enabled {
			return fmt.Errorf("%s.module references unavailable module %q", prefix, block.Module)
		}
	}
	for id, module := range cfg.Modules {
		if module.Animation != "" && !oneOf(module.Animation, "none", "glint", "pulse", "blink") {
			return fmt.Errorf("module.%s.animation has invalid value %q", id, module.Animation)
		}
		if module.AnimationIntervalMS < 0 {
			return fmt.Errorf("module.%s.animation_interval_ms must be >= 0", id)
		}
		if module.MaxWidth < 0 {
			return fmt.Errorf("module.%s.max_width must be >= 0", id)
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
		for _, name := range module.Env {
			if !validEnvName(name) {
				return fmt.Errorf("module.%s.env has invalid value %q", id, name)
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

var numericWidth = regexp.MustCompile(`^[1-9][0-9]*%?$`)
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validWidth(width string) bool {
	if width == "auto" || width == "fill" {
		return true
	}
	if !numericWidth.MatchString(width) {
		return false
	}
	if !strings.HasSuffix(width, "%") {
		return true
	}
	percent, _ := strconv.Atoi(strings.TrimSuffix(width, "%"))
	return percent <= 100
}

func validEnvName(name string) bool {
	return envName.MatchString(name)
}

// DefaultPath returns $XDG_CONFIG_HOME/ptyline/config.toml, falling back to
// ~/.config/ptyline/config.toml (spec §13).
func DefaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ptyline", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "ptyline.toml"
	}
	return filepath.Join(home, ".config", "ptyline", "config.toml")
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
		if meta.IsDefined("module", id, "provider") && mod.Provider == "command" {
			return fmt.Errorf("overlay must not set module.%s.provider = \"command\"", id)
		}
	}
	return nil
}

// MergeOverlay applies an overlay on top of base. Map sections (modules, styles,
// theme palette maps) merge by key; slice fields (env, bar.row, bar.block) replace
// entirely when the overlay defines them; scalar fields use the overlay value when
// non-zero; booleans use IsDefined to detect explicit sets.
func MergeOverlay(base Config, overlay Config, meta toml.MetaData) Config {
	result := base

	// Bar layout
	if overlay.Bar.Format != "" {
		result.Bar.Format = overlay.Bar.Format
	}
	if meta.IsDefined("bar", "row") {
		result.Bar.Rows = overlay.Bar.Rows
	}
	if meta.IsDefined("bar", "block") {
		result.Bar.Blocks = overlay.Bar.Blocks
	}
	if overlay.Bar.Separator != "" {
		result.Bar.Separator = overlay.Bar.Separator
	}
	if overlay.Bar.Padding != 0 {
		result.Bar.Padding = overlay.Bar.Padding
	}
	if overlay.Bar.Mode != "" {
		result.Bar.Mode = overlay.Bar.Mode
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
	if result.Theme.Agent == nil {
		result.Theme.Agent = map[string]string{}
	}
	for k, v := range overlay.Theme.Agent {
		result.Theme.Agent[k] = v
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
	if overlay.Format != "" {
		base.Format = overlay.Format
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
	if overlay.Animation != "" {
		base.Animation = overlay.Animation
	}
	if overlay.AnimationIntervalMS != 0 {
		base.AnimationIntervalMS = overlay.AnimationIntervalMS
	}
	if overlay.Icon != "" {
		base.Icon = overlay.Icon
	}
	if overlay.FallbackIcon != "" {
		base.FallbackIcon = overlay.FallbackIcon
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
	if overlay.Shape != "" {
		base.Shape = overlay.Shape
	}
	if overlay.LeftSeparator != "" {
		base.LeftSeparator = overlay.LeftSeparator
	}
	if overlay.RightSeparator != "" {
		base.RightSeparator = overlay.RightSeparator
	}
	if overlay.PaddingLeft != 0 {
		base.PaddingLeft = overlay.PaddingLeft
	}
	if overlay.PaddingRight != 0 {
		base.PaddingRight = overlay.PaddingRight
	}
	return base
}

var moduleRefRE = regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`)

// inferActiveModules enables any module referenced in the bar layout unless it
// was explicitly disabled via enabled=false in an overlay (tracked by the caller).
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
	for _, m := range moduleRefRE.FindAllStringSubmatch(cfg.Bar.Format, -1) {
		activate(m[1])
	}
	for _, row := range cfg.Bar.Rows {
		for _, m := range moduleRefRE.FindAllStringSubmatch(row.Format, -1) {
			activate(m[1])
		}
	}
	for _, block := range cfg.Bar.Blocks {
		activate(block.Module)
	}
}

// ApplyOverlays merges each overlay path (in order, highest precedence last)
// on top of base, then infers active modules. Empty paths are skipped.
func ApplyOverlays(base Config, paths ...string) (Config, error) {
	result := base
	explicitlyDisabled := map[string]bool{}

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
	return result, nil
}
