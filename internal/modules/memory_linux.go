//go:build linux

package modules

import (
	"context"
	"os"
)

const defaultMeminfoPath = "/proc/meminfo"

type linuxMemoryProvider struct {
	path string
}

func newMemoryProvider() sampler[MemorySample] {
	return linuxMemoryProvider{path: defaultMeminfoPath}
}

func (p linuxMemoryProvider) Probe(ctx context.Context) error {
	_, err := p.Sample(ctx)
	return err
}

func (p linuxMemoryProvider) Sample(ctx context.Context) (MemorySample, error) {
	select {
	case <-ctx.Done():
		return MemorySample{}, ctx.Err()
	default:
	}

	data, err := os.ReadFile(p.path)
	if err != nil {
		return MemorySample{}, err
	}
	return parseMeminfo(string(data))
}
