//go:build !linux

package modules

import "context"

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
