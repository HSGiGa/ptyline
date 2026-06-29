//go:build linux

package modules

import (
	"context"
	"os"
)

const defaultLoadavgPath = "/proc/loadavg"

type linuxLoadProvider struct {
	path string
}

func newLoadProvider() sampler[LoadSample] {
	return linuxLoadProvider{path: defaultLoadavgPath}
}

func (p linuxLoadProvider) Probe(ctx context.Context) error {
	_, err := p.Sample(ctx)
	return err
}

func (p linuxLoadProvider) Sample(ctx context.Context) (LoadSample, error) {
	select {
	case <-ctx.Done():
		return LoadSample{}, ctx.Err()
	default:
	}

	data, err := os.ReadFile(p.path)
	if err != nil {
		return LoadSample{}, err
	}
	return parseLoadavg(string(data))
}
