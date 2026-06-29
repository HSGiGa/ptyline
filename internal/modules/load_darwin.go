//go:build darwin

package modules

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"errors"
)

// darwinLoadProvider reports the host load average via libc getloadavg(3), the
// macOS counterpart to reading /proc/loadavg on Linux.
type darwinLoadProvider struct{}

func newLoadProvider() sampler[LoadSample] {
	return darwinLoadProvider{}
}

func (p darwinLoadProvider) Probe(ctx context.Context) error {
	_, err := p.Sample(ctx)
	return err
}

func (p darwinLoadProvider) Sample(ctx context.Context) (LoadSample, error) {
	select {
	case <-ctx.Done():
		return LoadSample{}, ctx.Err()
	default:
	}

	var avg [3]C.double
	if n := C.getloadavg(&avg[0], 3); n != 3 {
		return LoadSample{}, errors.New("getloadavg: unavailable")
	}
	return LoadSample{
		Load1:  float64(avg[0]),
		Load5:  float64(avg[1]),
		Load15: float64(avg[2]),
	}, nil
}
