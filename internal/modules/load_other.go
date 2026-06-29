//go:build !linux && !darwin

package modules

import (
	"context"
	"errors"
)

var errLoadUnavailable = errors.New("load provider unavailable")

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
