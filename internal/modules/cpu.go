package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

var errCPUUnavailable = errors.New("cpu provider unavailable")

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

// cpuProvider turns successive raw cpuTimes readings into a utilization
// percentage. It keeps the previous reading; Probe primes it so the first Sample
// yields a (near-zero) delta rather than hiding the module. read is the
// platform-specific source of cpuTimes.
//
// mu guards prev/hasPrev because Refresh can run concurrently with the next
// scheduled tick if a sample overruns its timeout (see scheduler).
type cpuProvider struct {
	mu      sync.Mutex
	read    func(ctx context.Context) (cpuTimes, error)
	prev    cpuTimes
	hasPrev bool
}

func (p *cpuProvider) Probe(ctx context.Context) error {
	t, err := p.read(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.prev = t
	p.hasPrev = true
	p.mu.Unlock()
	return nil
}

func (p *cpuProvider) Sample(ctx context.Context) (CPUSample, error) {
	t, err := p.read(ctx)
	if err != nil {
		return CPUSample{}, err
	}
	p.mu.Lock()
	prev, hasPrev := p.prev, p.hasPrev
	p.prev = t
	p.hasPrev = true
	p.mu.Unlock()

	if !hasPrev {
		return CPUSample{}, nil
	}
	return cpuPercent(prev, t), nil
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
	totalDelta := next.Total - prev.Total
	if totalDelta == 0 {
		return CPUSample{}
	}
	idleDelta := next.Idle - prev.Idle
	if idleDelta > totalDelta {
		idleDelta = totalDelta
	}
	return CPUSample{Percent: 100 * float64(totalDelta-idleDelta) / float64(totalDelta)}
}

func formatCPU(sample CPUSample, format string) string {
	return strings.ReplaceAll(format, "{percent}", fmt.Sprintf("%.0f", sample.Percent))
}
