package app

import (
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/modules"
	"github.com/hsgiga/ptyline/internal/status"
)

// Registers the {load} system module. Adding a metric is exactly this: one file
// with a build closure and default interval/timeout — no edits to shared code.
func init() {
	registerProbeMod(probeModSpec{
		id:              "load",
		defaultInterval: 5 * time.Second,
		defaultTimeout:  100 * time.Millisecond,
		build: func(c config.ModuleConfig, iv time.Duration, _ probeModDeps) status.ProbeModule {
			return modules.NewLoad(iv, c.Format)
		},
	})
}
