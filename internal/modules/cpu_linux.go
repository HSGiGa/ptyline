//go:build linux

package modules

import (
	"context"
	"os"
)

const defaultProcStatPath = "/proc/stat"

func newCPUProvider() sampler[CPUSample] {
	return &cpuProvider{read: readProcStat(defaultProcStatPath)}
}

// readProcStat returns a cpuTimes reader bound to path (/proc/stat).
func readProcStat(path string) func(context.Context) (cpuTimes, error) {
	return func(ctx context.Context) (cpuTimes, error) {
		select {
		case <-ctx.Done():
			return cpuTimes{}, ctx.Err()
		default:
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return cpuTimes{}, err
		}
		return parseProcStatCPU(string(data))
	}
}
