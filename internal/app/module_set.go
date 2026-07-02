package app

import (
	"context"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/modules"
	"github.com/hsgiga/ptyline/internal/status"
)

// userModSet manages the lifecycle of user-defined exec and custom-time modules.
// It owns the canonical map of running entries and can diff-reconcile them against
// a new config so start/stop/restart decisions are always in one place.
//
// It is only ever touched from the event-loop goroutine, so it needs no lock.
type userModSet struct {
	entries  map[string]*userModEntry
	execMods []*execModuleRuntime

	// deps wired at startup, read-only after init.
	execDeps  *execRuntimeDeps
	scheduler *status.Scheduler
	state     *status.StatusState
	ctx       context.Context
	bus       *event.Bus
}

func newUserModSet(ctx context.Context, scheduler *status.Scheduler, state *status.StatusState, bus *event.Bus, execDeps *execRuntimeDeps) *userModSet {
	return &userModSet{
		entries:   map[string]*userModEntry{},
		execDeps:  execDeps,
		scheduler: scheduler,
		state:     state,
		ctx:       ctx,
		bus:       bus,
	}
}

// ExecModules returns the current set of exec module runtimes, used to route
// refresh-on-command and refresh-on-cwd triggers.
func (ms *userModSet) ExecModules() []*execModuleRuntime { return ms.execMods }

// Start creates and launches a new module for id using mcfg. Callers must
// ensure id is not already running (Stop it first if needed).
func (ms *userModSet) Start(id string, mcfg config.ModuleConfig) {
	src := config.ModuleSource(id, mcfg)
	interval := moduleInterval(mcfg, time.Second)
	mCtx, mCancel := context.WithCancel(ms.ctx)
	switch src {
	case "exec":
		if mcfg.Command == "" {
			mCancel()
			return
		}
		em := newExecModuleRuntime(id, mcfg, ms.execDeps)
		em.start(mCtx, ms.bus)
		em.refresh(mCtx, ms.bus) // initial snapshot off-loop via bus
		ms.entries[id] = &userModEntry{cancel: mCancel, configKey: modConfigKey(id, mcfg), exec: em}
	case "time":
		tm := modules.NewTimeWithID(id, mcfg.Format, interval)
		ms.scheduler.Start(mCtx, tm, 2*time.Second)
		ms.state.UpdateModule(tm.Refresh(ms.ctx))
		ms.entries[id] = &userModEntry{cancel: mCancel, configKey: modConfigKey(id, mcfg)}
	default:
		mCancel()
	}
}

// Stop cancels the running context for id and removes it from the set.
// No-op when id is not present.
func (ms *userModSet) Stop(id string) {
	if e, ok := ms.entries[id]; ok {
		e.cancel()
		delete(ms.entries, id)
	}
}

// Reconcile diffs the current running set against the desired set derived from
// cfg. Modules that no longer exist are stopped; modules with changed config are
// restarted; unchanged modules are left running. After Reconcile, ExecModules()
// reflects the new set.
func (ms *userModSet) Reconcile(cfg config.Config) {
	desired := map[string]config.ModuleConfig{}
	for id, mcfg := range cfg.Modules {
		src := config.ModuleSource(id, mcfg)
		if (src == "exec" && mcfg.Command != "") || src == "time" {
			desired[id] = mcfg
		}
	}

	// Stop modules that have been removed.
	for id := range ms.entries {
		if _, ok := desired[id]; !ok {
			ms.Stop(id)
		}
	}

	// Restart changed modules; start new ones.
	for id, mcfg := range desired {
		if existing, ok := ms.entries[id]; ok && modConfigKey(id, mcfg) == existing.configKey {
			continue // no relevant config change — goroutine keeps running
		}
		ms.Stop(id)
		ms.Start(id, mcfg)
	}

	ms.rebuildExecList()
}

func (ms *userModSet) rebuildExecList() {
	em := ms.execMods[:0] // reuse backing array
	for _, entry := range ms.entries {
		if entry.exec != nil {
			em = append(em, entry.exec)
		}
	}
	ms.execMods = em
}
