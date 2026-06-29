//go:build windows

package modules

import "context"

type unavailableDiskProvider struct{}

func newDiskProvider(func() string) sampler[DiskSample] {
	return unavailableDiskProvider{}
}

func (unavailableDiskProvider) Probe(context.Context) error {
	return errDiskUnavailable
}

func (unavailableDiskProvider) Sample(context.Context) (DiskSample, error) {
	return DiskSample{}, errDiskUnavailable
}
