package app

import (
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/modules"
	"github.com/hsgiga/ptyline/internal/status"
)

func init() {
	registerProbeMod(probeModSpec{
		id:              "battery",
		defaultInterval: 30 * time.Second,
		defaultTimeout:  250 * time.Millisecond,
		build: func(c config.ModuleConfig, iv time.Duration, _ probeModDeps) status.ProbeModule {
			return modules.NewBattery(iv, c.Format)
		},
	})
}
