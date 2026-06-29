//go:build !linux && !darwin

package modules

import (
	"context"
	"errors"
)

var errCPUUnavailable = errors.New("cpu provider unavailable")

type unavailableCPUProvider struct{}

func newCPUProvider() sampler[CPUSample] {
	return unavailableCPUProvider{}
}

func (unavailableCPUProvider) Probe(context.Context) error {
	return errCPUUnavailable
}

func (unavailableCPUProvider) Sample(context.Context) (CPUSample, error) {
	return CPUSample{}, errCPUUnavailable
}
