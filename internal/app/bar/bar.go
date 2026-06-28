// Package bar bridges config to the renderer: it parses bar row configs into
// typed specs, computes bar geometry, and derives animation settings.
// It is the only place that knows both config and renderer; neither package
// needs to know the other.
package bar

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/reserved"
	"github.com/hsgiga/ptyline/internal/status"
	"github.com/hsgiga/ptyline/internal/status/icons"
	"github.com/hsgiga/ptyline/internal/status/layout"
	"github.com/hsgiga/ptyline/internal/status/renderer"
)

// RowSpec is one resolved bar row: its parsed blocks and gap/cap fill rune.
type RowSpec struct {
	Blocks    []layout.Block
	Fill      rune
	Separator string
}

// BuildRows resolves the configured bar into RowSpec values. Multi-line
// [[bar.row]] entries take precedence; otherwise the single-line Format becomes
// one space-filled row.
func BuildRows(cfg config.Config) []RowSpec {
	if len(cfg.Bar.Rows) > 0 {
		rows := make([]RowSpec, len(cfg.Bar.Rows))
		for i, rc := range cfg.Bar.Rows {
			fill := ' '
			if rc.Fill != "" {
				fill = []rune(rc.Fill)[0]
			}
			separator := cfg.Bar.Separator
			if rc.Separator != "" {
				separator = rc.Separator
			}
			rows[i] = RowSpec{Blocks: layout.ParseFormat(rc.Format), Fill: fill, Separator: separator}
		}
		return rows
	}
	return []RowSpec{{Blocks: layout.ParseFormat(cfg.Bar.Format), Fill: ' ', Separator: cfg.Bar.Separator}}
}

// TemplateSpecs builds the map of pre-parsed template module specs from config.
// The map is passed to the renderer so template values are resolved at render
// time from cached snapshots without any provider calls.
func TemplateSpecs(cfg config.Config) map[string]renderer.TemplateSpec {
	specs := map[string]renderer.TemplateSpec{}
	for id, mcfg := range cfg.Modules {
		if config.ModuleSource(id, mcfg) != "template" {
			continue
		}
		specs[id] = renderer.TemplateSpec{
			Blocks:             layout.ParseFormat(mcfg.Format),
			HideWhenEmpty:      mcfg.HideWhenEmpty,
			CollapseWhitespace: mcfg.CollapseWhitespace,
			MaxWidth:           mcfg.MaxWidth,
			Separator:          mcfg.Separator,
		}
	}
	return specs
}

// IconSpecs builds display-only module icon specs. Modules keep returning clean
// values; renderer attaches icons only to non-empty rendered module blocks.
func IconSpecs(cfg config.Config) map[string]renderer.ModuleIcon {
	specs := map[string]renderer.ModuleIcon{}
	if !cfg.Icons.Enabled {
		return specs
	}
	preset := icons.Preset(cfg.Icons.Preset)
	for id, mcfg := range cfg.Modules {
		if mcfg.Icon == "" {
			continue
		}
		glyph, fallback := moduleIconGlyphs(id, mcfg)
		text := resolveModuleIcon(preset, cfg.Icons.Fallback, glyph, fallback)
		if text == "" {
			continue
		}
		specs[id] = renderer.ModuleIcon{Position: mcfg.Icon, Text: text}
	}
	return specs
}

type moduleIconDefaults struct {
	glyph    string
	fallback string
}

var defaultModuleIcons = map[string]moduleIconDefaults{
	"command":  {glyph: "󰘳"},
	"cwd":      {glyph: "󰉋"},
	"env":      {glyph: "󰘓"},
	"git":      {glyph: "", fallback: "⎇"},
	"hostname": {glyph: "󰒋"},
	"runtime":  {glyph: "󰌠"},
	"shell":    {glyph: "󰆍"},
	"ssh":      {glyph: "󰣀", fallback: "ssh"},
	"time":     {glyph: "󰥔"},
	"user":     {glyph: ""},
}

func moduleIconGlyphs(id string, cfg config.ModuleConfig) (glyph, fallback string) {
	if defaults, ok := defaultModuleIcons[id]; ok {
		glyph, fallback = defaults.glyph, defaults.fallback
	}
	if cfg.IconGlyph != "" {
		glyph = cfg.IconGlyph
	}
	if cfg.IconFallback != "" {
		fallback = cfg.IconFallback
	}
	return glyph, fallback
}

func resolveModuleIcon(preset icons.Preset, fallbackEnabled bool, glyph, fallback string) string {
	if preset == icons.PresetNerdFont && glyph != "" {
		return glyph
	}
	if preset == icons.PresetASCII || fallbackEnabled {
		return fallback
	}
	return ""
}

// Render renders every RowSpec to a line, top to bottom.
func Render(r *renderer.Renderer, st status.StatusState, rows []RowSpec) []string {
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = r.RenderRow(st, row.Blocks, row.Fill, row.Separator).Line
	}
	return lines
}

// Geometry returns the 1-based first bar row and how many of the `want` rows
// actually fit above the child area; on a short terminal the bottom rows are
// dropped so the bar never paints past the last row (spec §15).
func Geometry(area reserved.Area, rows uint16, want int) (top uint16, count int) {
	child := area.ChildRows(rows)
	top = child + 1
	count = int(rows) - int(child)
	if count > want {
		count = want
	}
	if count < 0 {
		count = 0
	}
	return top, count
}

// AnimationsFromConfig converts module config entries to an Animation map for
// the renderer. Disabled or "none" animation modules are excluded.
func AnimationsFromConfig(modules map[string]config.ModuleConfig) map[string]renderer.Animation {
	animations := make(map[string]renderer.Animation)
	for id, module := range modules {
		if !module.Enabled || !module.Animation.Enabled() {
			continue
		}
		if id == "time" {
			continue
		}
		trigger := "change"
		if id == "command" {
			trigger = "active"
		}
		animations[id] = renderer.Animation{
			Mode:          module.Animation.Effect(),
			Trigger:       trigger,
			DurationTicks: durationTicks(module.AnimationIntervalMS),
		}
	}
	return animations
}

func durationTicks(intervalMS int) int {
	if intervalMS <= 0 {
		intervalMS = 120
	}
	ticks := 900 / intervalMS
	if ticks < 1 {
		return 1
	}
	return ticks
}

// TickerConfig derives the global tick interval and whether it is continuous
// (legacy only; current animations are gated by command/change activity).
func TickerConfig(modules map[string]config.ModuleConfig) (interval time.Duration, continuous bool) {
	for id, module := range modules {
		if !module.Enabled || !module.Animation.Enabled() || id == "time" {
			continue
		}
		next := time.Duration(module.AnimationIntervalMS) * time.Millisecond
		if next <= 0 {
			next = 120 * time.Millisecond
		}
		if interval == 0 || next < interval {
			interval = next
		}
	}
	return interval, continuous
}

// StartTicker launches the animation ticker goroutine. active is the
// command-animating flag; changeActive is raised by the renderer while
// value-change animations are still visible. Does nothing if interval <= 0.
func StartTicker(ctx context.Context, bus *event.Bus, modules map[string]config.ModuleConfig, active, changeActive *atomic.Bool) {
	interval, continuous := TickerConfig(modules)
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if continuous || (active != nil && active.Load()) || (changeActive != nil && changeActive.Load()) {
					bus.SendCtx(ctx, event.Tick{})
				}
			}
		}
	}()
}
