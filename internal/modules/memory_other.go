//go:build !linux && !darwin

package modules

import (
	"context"
	"errors"
)

var errMemoryUnavailable = errors.New("memory provider unavailable")

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
