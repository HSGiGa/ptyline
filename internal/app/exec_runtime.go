package app

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/event"
	"github.com/hsgiga/ptyline/internal/modules"
	"github.com/hsgiga/ptyline/internal/status"
)

// execRuntimeDeps supplies the live shell context an exec module runs against:
// env resolves a module's env patterns to name=value pairs from the shell's last
// reported snapshot, and cwd returns the shell's current working directory. Both
// are optional (nil in tests).
type execRuntimeDeps struct {
	env func([]string) []string
	cwd func() string
}

type execModuleRuntime struct {
	module           *modules.Exec
	refreshOnCommand []string
	refreshOnCWD     bool
	envProvider      func([]string) []string
	cwdProvider      func() string
	refreshing       atomic.Bool
	pending          atomic.Bool
}

func newExecModuleRuntime(id string, cfg config.ModuleConfig, deps *execRuntimeDeps) *execModuleRuntime {
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Second
	}
	r := &execModuleRuntime{
		module: modules.NewExec(
			id,
			cfg.Command,
			moduleInterval(cfg, 10*time.Second),
			timeout,
			cfg.Format,
			cfg.Separator,
			cfg.MaxWidth,
		).WithEnv(cfg.Env),
		refreshOnCommand: cfg.RefreshOnCommand,
		refreshOnCWD:     cfg.RefreshOnCWD,
	}
	if deps != nil {
		r.envProvider = deps.env
		r.cwdProvider = deps.cwd
	}
	return r
}

func (r *execModuleRuntime) start(ctx context.Context, bus *event.Bus) {
	if r.module.Interval() <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(r.module.Interval())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.refresh(ctx, bus)
			}
		}
	}()
}

// refresh runs the command off the event loop and emits a snapshot. Refreshes
// coalesce rather than drop: recording the request before trying to become the
// worker guarantees that if one is already running, it runs once more afterwards,
// so the final snapshot reflects the latest cwd/env (e.g. rapid `cd -`). At most
// one refresh runs at a time.
func (r *execModuleRuntime) refresh(ctx context.Context, bus *event.Bus) {
	if ctx.Err() != nil {
		return
	}
	r.pending.Store(true)
	if !r.refreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		// done is set on every normal exit (after refreshing was already cleared or
		// handed off). If we unwind without it — a panic in refreshSnapshot/SendCtx —
		// release the flag so the module isn't wedged with refreshing stuck true.
		done := false
		defer func() {
			if !done {
				r.refreshing.Store(false)
			}
		}()
		for {
			for r.pending.CompareAndSwap(true, false) {
				if ctx.Err() != nil {
					r.refreshing.Store(false)
					done = true
					return
				}
				rctx, cancel := context.WithTimeout(ctx, r.module.Timeout())
				snap := r.refreshSnapshot(rctx)
				cancel()
				bus.SendCtx(ctx, event.ModuleUpdated{ID: string(snap.ID), Snapshot: snap})
			}
			r.refreshing.Store(false)
			// A request may have arrived between the failed CAS above and clearing
			// the flag; re-acquire and loop, unless another caller already did.
			if !r.pending.Load() || !r.refreshing.CompareAndSwap(false, true) {
				done = true
				return
			}
		}
	}()
}

func (r *execModuleRuntime) refreshSnapshot(ctx context.Context) status.ModuleSnapshot {
	var env []string
	var dir string
	if r.envProvider != nil {
		env = r.envProvider(r.module.EnvNames())
	}
	if r.cwdProvider != nil {
		dir = r.cwdProvider()
	}
	return r.module.RefreshWithEnv(ctx, env, dir)
}

// mirrorsAny reports whether the module mirrors any of the given variable names,
// i.e. one of its env patterns (exact or prefix) matches. Used to refresh only
// affected modules when the shell's mirrored environment changes.
func (r *execModuleRuntime) mirrorsAny(names []string) bool {
	patterns := r.module.EnvNames()
	for _, name := range names {
		for _, pattern := range patterns {
			if envNameMatches(name, pattern) {
				return true
			}
		}
	}
	return false
}

func (r *execModuleRuntime) refreshAfterCommand(ctx context.Context, bus *event.Bus, command string) {
	for _, pattern := range r.refreshOnCommand {
		if commandMatches(command, pattern) {
			r.refresh(ctx, bus)
			return
		}
	}
}

func commandMatches(actual, pattern string) bool {
	actual = normalizeCommand(actual)
	pattern = normalizeCommand(pattern)
	if actual == "" || pattern == "" {
		return false
	}
	return actual == pattern || strings.HasPrefix(actual, pattern+" ")
}

func normalizeCommand(command string) string {
	return strings.Join(strings.Fields(command), " ")
}

func exitCodeSuccess(value string) bool {
	code, err := strconv.Atoi(value)
	return err == nil && code == 0
}

func shouldRefreshAfterExit(exitCode, pendingCommand, lastCommand string) bool {
	return exitCodeSuccess(exitCode) && pendingCommand != "" && pendingCommand == lastCommand
}
