package bar

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/status/style"
	"github.com/hsgiga/ptyline/internal/status/theme"
)

type Visuals struct {
	Theme  *theme.Theme
	Styles map[string]style.Style
}

type themeFile struct {
	Name    string                        `toml:"name"`
	Palette map[string]string             `toml:"palette"`
	Styles  map[string]config.StyleConfig `toml:"style"`
}

// VisualsFromConfig resolves the configured palette and styles into renderer
// inputs. The layering is: built-in defaults, optional theme file, inline
// [theme.*] overrides, then inline [style.*] overrides.
func VisualsFromConfig(cfg config.Config, mode theme.Mode, configPath string) (Visuals, error) {
	palette := theme.DefaultPalette()
	styles := map[string]style.Style{}

	if scheme := cfg.Theme.ColorScheme; scheme != "" && scheme != "default" {
		fileTheme, path, err := loadThemeFile(configPath, scheme)
		if err != nil {
			return Visuals{}, err
		}
		if fileTheme.Name != "" && fileTheme.Name != scheme {
			return Visuals{}, fmt.Errorf("%s: name = %q, want %q", path, fileTheme.Name, scheme)
		}
		if err := mergePalette(palette, fileTheme.Palette, path+": palette"); err != nil {
			return Visuals{}, err
		}
		if err := mergeStyles(styles, fileTheme.Styles, path+": style", palette); err != nil {
			return Visuals{}, err
		}
	}

	if err := mergePalette(palette, cfg.Theme.Palette, "theme.palette"); err != nil {
		return Visuals{}, err
	}
	if err := mergePalette(palette, cfg.Theme.Status, "theme.status"); err != nil {
		return Visuals{}, err
	}
	if err := mergePalette(palette, cfg.Theme.Agent, "theme.agent"); err != nil {
		return Visuals{}, err
	}
	if err := mergeStyles(styles, cfg.Styles, "style", palette); err != nil {
		return Visuals{}, err
	}

	return Visuals{Theme: theme.New(mode, palette), Styles: styles}, nil
}

func loadThemeFile(configPath, name string) (themeFile, string, error) {
	path := filepath.Join(filepath.Dir(config.ResolvePath(configPath)), "themes", name+".toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return themeFile{}, path, fmt.Errorf("%s: theme %q not found", path, name)
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
	return style.Style{
		FG:           cfg.FG,
		BG:           cfg.BG,
		Bold:         cfg.Bold,
		Dim:          cfg.Dim,
		Italic:       cfg.Italic,
		Underline:    cfg.Underline,
		Shape:        shape,
		LeftCap:      cfg.LeftCap,
		RightCap:     cfg.RightCap,
		PaddingLeft:  cfg.PaddingLeft,
		PaddingRight: cfg.PaddingRight,
	}, nil
}
