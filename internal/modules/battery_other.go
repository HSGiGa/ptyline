//go:build !linux && !darwin

package modules

import (
	"context"
	"errors"
)

var errBatteryUnavailable = errors.New("battery provider unavailable")

type unavailableBatteryProvider struct{}

func newBatteryProvider() sampler[BatterySample] {
	return unavailableBatteryProvider{}
}

func (unavailableBatteryProvider) Probe(context.Context) error {
	return errBatteryUnavailable
}

func (unavailableBatteryProvider) Sample(context.Context) (BatterySample, error) {
	return BatterySample{}, errBatteryUnavailable
}
