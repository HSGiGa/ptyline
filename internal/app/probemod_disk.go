package app

import (
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/modules"
	"github.com/hsgiga/ptyline/internal/status"
)

func init() {
	registerProbeMod(probeModSpec{
		id:              "disk",
		defaultInterval: time.Minute,
		defaultTimeout:  250 * time.Millisecond,
		refreshOnCWD:    true, // {disk} tracks the shell cwd; resample on cd
		build: func(c config.ModuleConfig, iv time.Duration, deps probeModDeps) status.ProbeModule {
			return modules.NewDisk(iv, c.Format, deps.cwd)
		},
	})
}
