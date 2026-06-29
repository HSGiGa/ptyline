package app

import (
	"context"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/status"
)

// probeModDeps carries the runtime dependencies a probe-module's build closure
// may need but cannot get from config alone (e.g. the live shell cwd).
type probeModDeps struct {
	// cwd reports the shell's current directory (tracked via shell integration);
	// nil-safe to call. Used by path-relative modules such as {disk}.
	cwd func() string
}

// probeModSpec declares one probe-driven system module. Each module feature
// registers exactly one spec via registerProbeMod (typically from an init in its
// own file, so features never edit a shared list). The manager owns everything
// after that: probe, scheduling, initial refresh, reload reconciliation.
type probeModSpec struct {
	id              string
	defaultInterval time.Duration
	defaultTimeout  time.Duration
	// build constructs the module from its resolved config, the interval the
	// manager computed, and runtime deps — so per-config state (format, cwd) is
	// always fresh.
	build func(cfg config.ModuleConfig, interval time.Duration, deps probeModDeps) status.ProbeModule
	// refreshOnCWD asks the manager to take an extra sample when the shell's
	// directory changes, for path-relative metrics like {disk} that would
	// otherwise stay stale until the next interval tick.
	refreshOnCWD bool
}

// probeModRegistry holds every registered system-module spec. Populated at
// package init time by registerProbeMod; read once per run when the manager is
// built. Registration order is irrelevant — the bar layout is template-driven.
var probeModRegistry []probeModSpec

// registerProbeMod adds a system-module spec to the registry. Call it from an
// init in the module's own file; this keeps adding a metric to a single new
// file with no edits to shared code.
func registerProbeMod(spec probeModSpec) {
	probeModRegistry = append(probeModRegistry, spec)
}

// probeModEntry is the running state of one managed module. interval+format are
// the values its goroutine was started with, so Reconcile can detect changes;
// mod/ctx/timeout let the manager trigger an extra off-loop refresh on demand
// (see OnCWDChange).
type probeModEntry struct {
	cancel   context.CancelFunc
	interval time.Duration
	format   string
	mod      status.ProbeModule
	ctx      context.Context
	timeout  time.Duration
}

// probeModManager reconciles the set of running probe-modules against config,
// mirroring the user-module reconcile pattern (see updateModules): an enabled,
// unchanged module keeps running; a disabled one is cancelled; a changed one is
// restarted. Probe runs before scheduling, and the initial refresh runs off the
// event loop so slow sampler I/O can never block startup or reload.
//
// All methods must be called from the event-loop goroutine: they mutate the
// shared entries map and rely on the scheduler's emit path (a bus event) to be
// the only writer of StatusState.
type probeModManager struct {
	parent    context.Context
	scheduler *status.Scheduler
	deps      probeModDeps
	specs     []probeModSpec
	entries   map[string]*probeModEntry
}

func newProbeModManager(parent context.Context, scheduler *status.Scheduler, deps probeModDeps, specs []probeModSpec) *probeModManager {
	return &probeModManager{
		parent:    parent,
		scheduler: scheduler,
		deps:      deps,
		specs:     specs,
		entries:   map[string]*probeModEntry{},
	}
}

// Reconcile brings the running set in line with cfg. Safe to call at startup and
// on every reload; it is a no-op for modules whose enabled/interval/format are
// unchanged.
func (mgr *probeModManager) Reconcile(cfg config.Config) {
	for _, spec := range mgr.specs {
		mgr.reconcileOne(spec, cfg.Modules[spec.id])
	}
}

func (mgr *probeModManager) reconcileOne(spec probeModSpec, mcfg config.ModuleConfig) {
	interval := moduleInterval(mcfg, spec.defaultInterval)
	running := mgr.entries[spec.id]

	// Disabled: stop any running goroutine and drop the entry.
	if !mcfg.Enabled {
		mgr.stop(spec.id)
		return
	}
	// Enabled and unchanged: leave the running goroutine alone.
	if running != nil && running.interval == interval && running.format == mcfg.Format {
		return
	}
	// Enabled but new or changed: restart from a clean slate.
	mgr.stop(spec.id)
	mgr.start(spec, mcfg, interval)
}

// start probes the module; on Available it schedules interval refreshes and
// kicks off one immediate off-loop refresh, recording the entry. On unavailable
// probe nothing is scheduled and no entry is recorded ("hidden, no polling").
func (mgr *probeModManager) start(spec probeModSpec, mcfg config.ModuleConfig, interval time.Duration) {
	timeout := moduleTimeout(mcfg, spec.defaultTimeout)
	mod := spec.build(mcfg, interval, mgr.deps)

	mCtx, cancel := context.WithCancel(mgr.parent)
	pctx, pcancel := context.WithTimeout(mCtx, timeout)
	probe := mod.Probe(pctx)
	pcancel()
	if !probe.Available {
		cancel()
		return
	}

	mgr.scheduler.Start(mCtx, mod, timeout)
	mgr.scheduler.RefreshOnce(mCtx, mod, timeout)
	mgr.entries[spec.id] = &probeModEntry{
		cancel:   cancel,
		interval: interval,
		format:   mcfg.Format,
		mod:      mod,
		ctx:      mCtx,
		timeout:  timeout,
	}
}

// stop cancels a running module's goroutines and forgets it. No-op if absent.
func (mgr *probeModManager) stop(id string) {
	if e := mgr.entries[id]; e != nil {
		e.cancel()
		delete(mgr.entries, id)
	}
}

// OnCWDChange takes an extra off-loop sample of every running module whose spec
// has refreshOnCWD set, so path-relative metrics (e.g. {disk}) update promptly
// after the shell changes directory instead of waiting for the next tick. A
// disabled or unavailable module has no entry and is skipped.
func (mgr *probeModManager) OnCWDChange() {
	for _, spec := range mgr.specs {
		if !spec.refreshOnCWD {
			continue
		}
		if e := mgr.entries[spec.id]; e != nil {
			mgr.scheduler.RefreshOnce(e.ctx, e.mod, e.timeout)
		}
	}
}
