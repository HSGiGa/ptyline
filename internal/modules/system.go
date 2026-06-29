package modules

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// percentWidth is the fixed cell width every {percent} value renders to, so the
// bar never reflows as a metric crosses a digit boundary (e.g. 9% -> 10%).
const percentWidth = 2

// formatPercent renders a percentage at a fixed width so the bar stays steady.
// The value is rounded and clamped to 0..99: capping at 99 keeps it two digits
// (a momentary 100% would otherwise widen the field and shove the layout), and
// 0..99 is enough resolution for an at-a-glance metric. The result is
// space-padded on the left to percentWidth (" 0", " 9", "99").
func formatPercent(v float64) string {
	n := int(math.Round(v))
	if n < 0 {
		n = 0
	}
	if n > 99 {
		n = 99
	}
	s := strconv.Itoa(n)
	for len(s) < percentWidth {
		s = " " + s
	}
	return s
}

// sampler is the platform-specific half of a system module: it reports whether
// the metric is available on this host (Probe) and produces one typed reading
// (Sample). All host I/O lives here; everything above it is uniform.
//
// Probe may prime per-sampler state needed for the first Sample (e.g. the CPU
// sampler stores a baseline reading, the battery sampler caches the discovered
// power-supply path). Implementations that hold mutable state across Sample
// calls must guard it themselves — Refresh can run concurrently with the next
// scheduled tick when a sample overruns its timeout (see scheduler).
type sampler[S any] interface {
	Probe(ctx context.Context) error
	Sample(ctx context.Context) (S, error)
}

// systemModule is the generic, uniform implementation shared by every system
// metric ({load}, {cpu}, {memory}, {battery}, {disk}). It owns the boilerplate
// — ID, interval, probe/refresh plumbing, snapshot construction — so each metric
// only supplies a sampler and a pure formatter. This is the single place that
// turns a reading into a ModuleSnapshot, keeping behavior identical across
// modules (spec §8.7).
type systemModule[S any] struct {
	id       status.ModuleID
	interval time.Duration
	format   string
	sampler  sampler[S]
	render   func(sample S, format string) string
}

// newSystemModule wires a metric together. defaultFormat is used when the user
// leaves format empty.
func newSystemModule[S any](
	id status.ModuleID,
	interval time.Duration,
	format, defaultFormat string,
	s sampler[S],
	render func(S, string) string,
) *systemModule[S] {
	if format == "" {
		format = defaultFormat
	}
	return &systemModule[S]{id: id, interval: interval, format: format, sampler: s, render: render}
}

func (m *systemModule[S]) ID() status.ModuleID     { return m.id }
func (m *systemModule[S]) Interval() time.Duration { return m.interval }

// Probe reports availability. An unavailable probe tells the app to hide the
// module and never poll it (spec §24, docs/features/system-modules.md).
func (m *systemModule[S]) Probe(ctx context.Context) status.ModuleProbe {
	if err := m.sampler.Probe(ctx); err != nil {
		return status.UnavailableProbe(err)
	}
	return status.AvailableProbe()
}

// Refresh takes one reading under ctx's deadline. A sampling error after a
// successful probe yields a stale/errored snapshot (the renderer keeps the last
// good value); a success yields a formatted text snapshot.
func (m *systemModule[S]) Refresh(ctx context.Context) status.ModuleSnapshot {
	sample, err := m.sampler.Sample(ctx)
	if err != nil {
		return status.ModuleSnapshot{ID: m.id, Stale: true, Err: err, UpdatedAt: time.Now()}
	}
	return status.ModuleSnapshot{
		ID:        m.id,
		Value:     status.Text(m.render(sample, m.format)),
		UpdatedAt: time.Now(),
	}
}
