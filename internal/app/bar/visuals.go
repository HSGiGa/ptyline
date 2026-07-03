package bar

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	assets "github.com/hsgiga/ptyline/config"
	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/status/style"
	"github.com/hsgiga/ptyline/internal/status/theme"
)

type Visuals struct {
	Theme  *theme.Theme
	Styles map[string]style.Style
}

// themeFile is a color scheme: a palette plus, for self-contained legacy and
// shell-default themes, its own [style.*] blocks. Palette-only themes leave
// Styles empty and take their shape from the style preset (styles/<name>.toml).
type themeFile struct {
	Name    string                        `toml:"name"`
	Palette map[string]string             `toml:"palette"`
	Styles  map[string]config.StyleConfig `toml:"style"`
}

// styleFile is a style preset: shape and per-block presentation only, resolved
// against whatever palette the color scheme provides.
type styleFile struct {
	Styles map[string]config.StyleConfig `toml:"style"`
}

// VisualsFromConfig resolves the configured palette and styles into renderer
// inputs. The layering, weakest first, is: built-in defaults, the color
// scheme's palette, inline [theme.*] palette overrides, the style preset's
// [style.*], the color scheme's own [style.*] (self-contained themes), then
// inline [style.*].
//
// Both color_scheme and theme.style accept "default" (or empty), which resolves
// per shell: fish/zsh/bash select fish-default/zsh-default/bash-default palettes
// and flat/powerline/flat style presets. An explicit value is used verbatim.
// When a default-derived file is missing, rendering falls back to the built-in
// terminal-native look (and to the flat preset, which every palette can back);
// an explicit name that is missing is an error.
func VisualsFromConfig(cfg config.Config, mode theme.Mode, configPath, shell string) (Visuals, error) {
	palette := theme.DefaultPalette()
	styles := map[string]style.Style{}

	scheme := cfg.Theme.ColorScheme
	schemeDerived := scheme == "" || scheme == "default"
	if schemeDerived {
		scheme = defaultScheme(shell)
	}

	fileTheme, path, err := loadThemeFile(configPath, scheme)
	paletteFellBack := false
	switch {
	case err == nil:
		if fileTheme.Name != "" && fileTheme.Name != scheme {
			return Visuals{}, fmt.Errorf("%s: name = %q, want %q", path, fileTheme.Name, scheme)
		}
		if err := mergePalette(palette, fileTheme.Palette, path+": palette"); err != nil {
			return Visuals{}, err
		}
	case schemeDerived && errors.Is(err, os.ErrNotExist):
		// No default theme installed for this shell: keep the terminal-native
		// palette. fileTheme stays zero, so it contributes no embedded styles.
		fileTheme = themeFile{}
		paletteFellBack = true
	default:
		return Visuals{}, err
	}

	if err := mergePalette(palette, cfg.Theme.Palette, "theme.palette"); err != nil {
		return Visuals{}, err
	}
	if err := mergePalette(palette, cfg.Theme.Status, "theme.status"); err != nil {
		return Visuals{}, err
	}

	styleName := cfg.Theme.Style
	styleDerived := styleName == "" || styleName == "default"
	if styleDerived {
		styleName = defaultStyle(shell)
		// The terminal-native fallback palette lacks base.bg/panel, so a
		// segmented preset cannot render on it; keep the token-safe flat preset.
		if paletteFellBack {
			styleName = "flat"
		}
	}

	if err := applyStylePreset(styles, configPath, styleName, styleDerived, palette); err != nil {
		return Visuals{}, err
	}
	if err := mergeStyles(styles, fileTheme.Styles, path+": style", palette); err != nil {
		return Visuals{}, err
	}
	if err := mergeStyles(styles, cfg.Styles, "style", palette); err != nil {
		return Visuals{}, err
	}

	return Visuals{Theme: theme.New(mode, palette), Styles: styles}, nil
}

// defaultScheme maps the interactive shell to its built-in default palette, and
// defaultStyle to its default style preset. These are the only places Go code
// names a shell; every other shell reference lives in the integration templates.
func defaultScheme(shell string) string {
	switch shell {
	case "fish":
		return "fish-default"
	case "zsh":
		return "zsh-default"
	default: // bash, sh, unknown, and command fallbacks
		return "bash-default"
	}
}

func defaultStyle(shell string) string {
	if shell == "zsh" {
		return "powerline" // matches the p10k/oh-my-zsh segment convention
	}
	return "flat"
}

// applyStylePreset merges styles/<name>.toml into dst. When derived (the name
// came from default resolution), a missing file falls back to the renderer's
// built-in styles rather than failing; a missing explicit preset is an error.
func applyStylePreset(dst map[string]style.Style, configPath, name string, derived bool, palette map[string]theme.RGB) error {
	preset, path, err := loadStyleFile(configPath, name)
	switch {
	case err == nil:
		return mergeStyles(dst, preset.Styles, path+": style", palette)
	case derived && errors.Is(err, os.ErrNotExist):
		return nil
	default:
		return err
	}
}

// readAsset returns the bytes for <subdir>/<name>.toml, preferring a file in the
// user's config directory and falling back to the copy embedded in the binary.
// The returned label identifies the source for diagnostics. A name present in
// neither place yields os.ErrNotExist wrapped with the on-disk path a user would
// create to add it.
func readAsset(configPath, subdir, name string, builtin embed.FS) ([]byte, string, error) {
	diskPath := filepath.Join(filepath.Dir(config.ResolvePath(configPath)), subdir, name+".toml")
	switch raw, err := os.ReadFile(diskPath); {
	case err == nil:
		return raw, diskPath, nil
	case !os.IsNotExist(err):
		return nil, diskPath, err
	}
	if raw, err := builtin.ReadFile(subdir + "/" + name + ".toml"); err == nil {
		return raw, "built-in " + subdir + "/" + name, nil
	}
	return nil, diskPath, os.ErrNotExist
}

func loadThemeFile(configPath, name string) (themeFile, string, error) {
	raw, path, err := readAsset(configPath, "themes", name, assets.Themes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return themeFile{}, path, fmt.Errorf("%s: theme %q not found: %w", path, name, os.ErrNotExist)
		}
		return themeFile{}, path, err
	}
	var file themeFile
	metadata, err := toml.Decode(string(raw), &file)
	if err != nil {
		return themeFile{}, path, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return themeFile{}, path, fmt.Errorf("%s: unknown key %q", path, undecoded[0])
	}
	return file, path, nil
}

// loadStyleFile reads styles/<name>.toml. A missing file is returned wrapping
// os.ErrNotExist so callers can distinguish it from a malformed preset.
func loadStyleFile(configPath, name string) (styleFile, string, error) {
	raw, path, err := readAsset(configPath, "styles", name, assets.Styles)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return styleFile{}, path, fmt.Errorf("%s: style preset %q not found: %w", path, name, os.ErrNotExist)
		}
		return styleFile{}, path, err
	}
	var file styleFile
	metadata, err := toml.Decode(string(raw), &file)
	if err != nil {
		return styleFile{}, path, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return styleFile{}, path, fmt.Errorf("%s: unknown key %q", path, undecoded[0])
	}
	return file, path, nil
}

func mergePalette(dst map[string]theme.RGB, src map[string]string, context string) error {
	for key, ref := range src {
		color, ok := resolvePaletteRef(ref, dst, src, map[string]bool{})
		if !ok {
			return fmt.Errorf("%s.%s has invalid color %q", context, key, ref)
		}
		dst[key] = color
	}
	return nil
}

func resolvePaletteRef(ref string, base map[string]theme.RGB, overrides map[string]string, seen map[string]bool) (theme.RGB, bool) {
	if next, ok := overrides[ref]; ok {
		if seen[ref] {
			return theme.RGB{}, false
		}
		seen[ref] = true
		return resolvePaletteRef(next, base, overrides, seen)
	}
	return theme.ResolveInPalette(ref, base)
}

func mergeStyles(dst map[string]style.Style, src map[string]config.StyleConfig, context string, palette map[string]theme.RGB) error {
	for id, cfg := range src {
		s, err := styleFromConfig(cfg, fmt.Sprintf("%s.%s", context, id), palette)
		if err != nil {
			return err
		}
		dst[id] = s
	}
	return nil
}

func styleFromConfig(cfg config.StyleConfig, context string, palette map[string]theme.RGB) (style.Style, error) {
	if cfg.FG != "" {
		if _, ok := theme.ResolveInPalette(cfg.FG, palette); !ok {
			return style.Style{}, fmt.Errorf("%s.fg has invalid color %q", context, cfg.FG)
		}
	}
	if cfg.BG != "" {
		if _, ok := theme.ResolveInPalette(cfg.BG, palette); !ok {
			return style.Style{}, fmt.Errorf("%s.bg has invalid color %q", context, cfg.BG)
		}
	}
	shape := style.Shape(cfg.Shape)
	if shape == "" {
		shape = style.ShapeFlat
	}
	switch shape {
	case style.ShapeFlat, style.ShapePowerline, style.ShapePill, style.ShapeBox:
	default:
		return style.Style{}, fmt.Errorf("%s.shape has invalid value %q", context, cfg.Shape)
	}
	if cfg.PaddingLeft < 0 {
		return style.Style{}, fmt.Errorf("%s.padding_left must be >= 0", context)
	}
	if cfg.PaddingRight < 0 {
		return style.Style{}, fmt.Errorf("%s.padding_right must be >= 0", context)
	}
	if cfg.Animation != "" && cfg.Animation != "glint" && cfg.Animation != "pulse" && cfg.Animation != "blink" {
		return style.Style{}, fmt.Errorf("%s.animation has invalid value %q", context, cfg.Animation)
	}
	return style.Style{
		FG:           cfg.FG,
		BG:           cfg.BG,
		Bold:         cfg.Bold,
		Dim:          cfg.Dim,
		Italic:       cfg.Italic,
		Underline:    cfg.Underline,
		Animation:    cfg.Animation,
		Shape:        shape,
		LeftCap:      cfg.LeftCap,
		RightCap:     cfg.RightCap,
		PaddingLeft:  cfg.PaddingLeft,
		PaddingRight: cfg.PaddingRight,
	}, nil
}
