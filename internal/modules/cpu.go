package modules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

type cpuTimes struct {
	Idle  uint64
	Total uint64
}

// CPUSample is one computed CPU utilization reading.
type CPUSample struct {
	Percent float64
}

// NewCPU builds the {cpu} system module (total host CPU utilization).
func NewCPU(interval time.Duration, format string) status.ProbeModule {
	return newSystemModule("cpu", interval, format, "cpu {percent}%", newCPUProvider(), formatCPU)
}

func parseProcStatCPU(data string) (cpuTimes, error) {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "cpu" {
			continue
		}
		if len(fields) < 5 {
			return cpuTimes{}, fmt.Errorf("parse cpu stat: got %d fields", len(fields))
		}
		var total uint64
		var idle uint64
		for i, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return cpuTimes{}, fmt.Errorf("parse cpu field %d: %w", i, err)
			}
			total += value
			if i == 3 || i == 4 {
				idle += value
			}
		}
		return cpuTimes{Idle: idle, Total: total}, nil
	}
	return cpuTimes{}, errors.New("parse cpu stat: missing aggregate cpu line")
}

func cpuPercent(prev, next cpuTimes) CPUSample {
	// Guard against a non-monotonic total (CPU hotplug changing the field set, or
	// a counter reset across suspend): report no measurable delta rather than
	// underflowing the uint64 subtraction into a bogus huge value. The == case
	// (no jiffies elapsed) is folded in here too.
	if next.Total <= prev.Total {
		return CPUSample{}
	}
	totalDelta := next.Total - prev.Total
	// idle can briefly regress on the same guard conditions; clamp so the busy
	// fraction stays in [0, 100].
	idleDelta := next.Idle - prev.Idle
	if idleDelta > totalDelta {
		idleDelta = totalDelta
	}
	return CPUSample{Percent: 100 * float64(totalDelta-idleDelta) / float64(totalDelta)}
}

func formatCPU(sample CPUSample, format string) string {
	return strings.ReplaceAll(format, "{percent}", fmt.Sprintf("%.0f", sample.Percent))
}
