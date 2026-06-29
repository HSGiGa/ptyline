//go:build linux || darwin

package modules

import (
	"context"
	"sync"
)

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
