//go:build !linux

package modules

import "context"

type unavailableLoadProvider struct{}

func newLoadProvider() sampler[LoadSample] {
	return unavailableLoadProvider{}
}

func (unavailableLoadProvider) Probe(context.Context) error {
	return errLoadUnavailable
}

func (unavailableLoadProvider) Sample(context.Context) (LoadSample, error) {
	return LoadSample{}, errLoadUnavailable
}
