package bar

import (
	"testing"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/reserved"
)

func TestGeometry(t *testing.T) {
	cases := []struct {
		name      string
		area      reserved.Area
		rows      uint16
		wantRows  int
		wantTop   uint16
		wantCount int
	}{
		{
			name:      "single bottom row",
			area:      reserved.Default(),
			rows:      30,
			wantRows:  1,
			wantTop:   30,
			wantCount: 1,
		},
		{
			name:      "multi row bar",
			area:      reserved.Area{Edge: reserved.Bottom, Rows: 2},
			rows:      30,
			wantRows:  2,
			wantTop:   29,
			wantCount: 2,
		},
		{
			name:      "tiny terminal drops overflowing bar rows",
			area:      reserved.Area{Edge: reserved.Bottom, Rows: 2},
			rows:      1,
			wantRows:  2,
			wantTop:   2,
			wantCount: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			top, count := Geometry(c.area, c.rows, c.wantRows)
			if top != c.wantTop || count != c.wantCount {
				t.Fatalf("Geometry(%+v, %d, %d) = (%d, %d), want (%d, %d)",
					c.area, c.rows, c.wantRows, top, count, c.wantTop, c.wantCount)
			}
		})
	}
}

func TestTickerConfig(t *testing.T) {
	interval, continuous := TickerConfig(map[string]config.ModuleConfig{
		"command": {Enabled: true, Animation: "glint", AnimationIntervalMS: 80},
	})
	if interval != 80*time.Millisecond || continuous {
		t.Fatalf("command animation = (%v, %t), want (80ms, false)", interval, continuous)
	}

	interval, continuous = TickerConfig(map[string]config.ModuleConfig{
		"git": {Enabled: true, Animation: config.AnimationDefault, AnimationIntervalMS: 120},
	})
	if interval != 120*time.Millisecond || continuous {
		t.Fatalf("git change animation = (%v, %t), want (120ms, false)", interval, continuous)
	}

	interval, continuous = TickerConfig(map[string]config.ModuleConfig{
		"time": {Enabled: true, Animation: config.AnimationDefault, AnimationIntervalMS: 120},
	})
	if interval != 0 || continuous {
		t.Fatalf("time animation = (%v, %t), want disabled", interval, continuous)
	}

	// animation_interval_ms unset: command falls back to its own faster 80ms
	// cadence, everything else falls back to the generic 120ms.
	interval, continuous = TickerConfig(map[string]config.ModuleConfig{
		"command": {Enabled: true, Animation: "glint"},
	})
	if interval != 80*time.Millisecond || continuous {
		t.Fatalf("command animation fallback = (%v, %t), want (80ms, false)", interval, continuous)
	}
	interval, continuous = TickerConfig(map[string]config.ModuleConfig{
		"git": {Enabled: true, Animation: config.AnimationDefault},
	})
	if interval != 120*time.Millisecond || continuous {
		t.Fatalf("git animation fallback = (%v, %t), want (120ms, false)", interval, continuous)
	}
}

func TestAnimationsFromConfig(t *testing.T) {
	got := AnimationsFromConfig(map[string]config.ModuleConfig{
		"command": {Enabled: true, Animation: config.AnimationDefault},
		"git":     {Enabled: true, Animation: "blink"},
		"time":    {Enabled: true, Animation: config.AnimationDefault},
		"env":     {Enabled: true, Animation: "none"},
	})
	if got["command"].Trigger != "active" || got["command"].Mode != "" {
		t.Fatalf("command animation = %+v, want default active", got["command"])
	}
	if got["git"].Trigger != "change" || got["git"].Mode != "blink" {
		t.Fatalf("git animation = %+v, want blink change", got["git"])
	}
	if _, ok := got["time"]; ok {
		t.Fatalf("time animation unexpectedly present: %+v", got)
	}
	if _, ok := got["env"]; ok {
		t.Fatalf("disabled animation unexpectedly present: %+v", got)
	}
}

func TestIconSpecsGitDefaultGlyphs(t *testing.T) {
	cfg := config.Default()
	cfg.Modules["git"] = config.ModuleConfig{Enabled: true, Icon: "left"}

	cfg.Icons.Preset = "nerd-font"
	got := IconSpecs(cfg)
	if got["git"].Position != "left" || got["git"].Text != "" {
		t.Fatalf("nerd git icon = %+v, want left ", got["git"])
	}

	cfg.Icons.Preset = "ascii"
	got = IconSpecs(cfg)
	if got["git"].Position != "left" || got["git"].Text != "⎇" {
		t.Fatalf("ascii git icon = %+v, want left ⎇", got["git"])
	}
}

func TestIconSpecsCustomExecGlyph(t *testing.T) {
	cfg := config.Default()
	cfg.Icons.Preset = "nerd-font"
	cfg.Modules["gh"] = config.ModuleConfig{
		Enabled:      true,
		Source:       "exec",
		Icon:         "right",
		IconGlyph:    "",
		IconFallback: "gh",
	}

	got := IconSpecs(cfg)
	if got["gh"].Position != "right" || got["gh"].Text != "" {
		t.Fatalf("custom exec icon = %+v, want right ", got["gh"])
	}
}
