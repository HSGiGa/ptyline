package app

import (
	"time"

	"github.com/hsgiga/ptyline/internal/config"
	"github.com/hsgiga/ptyline/internal/modules"
	"github.com/hsgiga/ptyline/internal/status"
)

func init() {
	registerProbeMod(probeModSpec{
		id:              "cpu",
		defaultInterval: 2 * time.Second,
		defaultTimeout:  100 * time.Millisecond,
		build: func(c config.ModuleConfig, iv time.Duration, _ probeModDeps) status.ProbeModule {
			return modules.NewCPU(iv, c.Format)
		},
	})
}
