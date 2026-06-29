//go:build !linux

package modules

import "context"

type unavailableMemoryProvider struct{}

func newMemoryProvider() sampler[MemorySample] {
	return unavailableMemoryProvider{}
}

func (unavailableMemoryProvider) Probe(context.Context) error {
	return errMemoryUnavailable
}

func (unavailableMemoryProvider) Sample(context.Context) (MemorySample, error) {
	return MemorySample{}, errMemoryUnavailable
}
