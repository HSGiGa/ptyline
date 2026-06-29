//go:build !linux

package modules

import "context"

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
