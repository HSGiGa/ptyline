package app

import (
	"testing"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/reserved"
)

func TestBarGeometry(t *testing.T) {
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
			top, count := barGeometry(c.area, c.rows, c.wantRows)
			if top != c.wantTop || count != c.wantCount {
				t.Fatalf("barGeometry(%+v, %d, %d) = (%d, %d), want (%d, %d)",
					c.area, c.rows, c.wantRows, top, count, c.wantTop, c.wantCount)
			}
		})
	}
}

func TestAnimationTickerConfig(t *testing.T) {
	interval, continuous := animationTickerConfig(map[string]config.ModuleConfig{
		"command": {Enabled: true, Animation: "glint", AnimationIntervalMS: 80},
	})
	if interval != 80*time.Millisecond || continuous {
		t.Fatalf("command animation = (%v, %t), want (80ms, false)", interval, continuous)
	}

	interval, continuous = animationTickerConfig(map[string]config.ModuleConfig{
		"time": {Enabled: true, Animation: "glint", AnimationIntervalMS: 120},
	})
	if interval != 120*time.Millisecond || !continuous {
		t.Fatalf("time animation = (%v, %t), want (120ms, true)", interval, continuous)
	}
}

func TestAnimationsFromConfig(t *testing.T) {
	got := animationsFromConfig(map[string]config.ModuleConfig{
		"time": {Enabled: true, Animation: "glint"},
		"git":  {Enabled: true, Animation: "none"},
	})
	if got["time"].Mode != "glint" {
		t.Fatalf("time animation = %+v, want glint", got["time"])
	}
	if _, ok := got["git"]; ok {
		t.Fatalf("disabled animation unexpectedly present: %+v", got)
	}
}
