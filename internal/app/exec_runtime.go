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
)

type execModuleRuntime struct {
	module           *modules.Exec
	refreshOnCommand []string
	refreshing       atomic.Bool
}

func newExecModuleRuntime(id string, cfg config.ModuleConfig) *execModuleRuntime {
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Second
	}
	return &execModuleRuntime{
		module: modules.NewExec(
			id,
			cfg.Command,
			moduleInterval(cfg, 10*time.Second),
			timeout,
			cfg.Format,
			cfg.MaxWidth,
		),
		refreshOnCommand: cfg.RefreshOnCommand,
	}
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

func (r *execModuleRuntime) refresh(ctx context.Context, bus *event.Bus) {
	if ctx.Err() != nil {
		return
	}
	if !r.refreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer r.refreshing.Store(false)
		rctx, cancel := context.WithTimeout(ctx, r.module.Timeout())
		defer cancel()
		snap := r.module.Refresh(rctx)
		bus.SendCtx(ctx, event.ModuleUpdated{ID: string(snap.ID), Snapshot: snap})
	}()
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
