//go:build linux

package modules

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const defaultPowerSupplyRoot = "/sys/class/power_supply"

// linuxBatteryProvider reads battery state from sysfs. The discovered power-supply
// directory is cached after Probe; mu guards it so a sample that overruns its
// timeout cannot race the next tick's lazy rediscovery.
type linuxBatteryProvider struct {
	root string

	mu      sync.Mutex
	battery string
}

func newBatteryProvider() sampler[BatterySample] {
	return &linuxBatteryProvider{root: defaultPowerSupplyRoot}
}

func (p *linuxBatteryProvider) Probe(ctx context.Context) error {
	battery, err := p.findBattery(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.battery = battery
	p.mu.Unlock()
	return nil
}

func (p *linuxBatteryProvider) Sample(ctx context.Context) (BatterySample, error) {
	p.mu.Lock()
	battery := p.battery
	p.mu.Unlock()
	if battery == "" {
		var err error
		if battery, err = p.findBattery(ctx); err != nil {
			return BatterySample{}, err
		}
		p.mu.Lock()
		p.battery = battery
		p.mu.Unlock()
	}

	select {
	case <-ctx.Done():
		return BatterySample{}, ctx.Err()
	default:
	}

	capacityData, err := os.ReadFile(filepath.Join(battery, "capacity"))
	if err != nil {
		return BatterySample{}, err
	}
	percent, err := parseBatteryCapacity(string(capacityData))
	if err != nil {
		return BatterySample{}, err
	}

	state := "unknown"
	if statusData, err := os.ReadFile(filepath.Join(battery, "status")); err == nil {
		state = normalizeBatteryState(string(statusData))
	}
	return BatterySample{Percent: percent, State: state}, nil
}

func (p *linuxBatteryProvider) findBattery(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	entries, err := os.ReadDir(p.root)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(p.root, entry.Name())
		if !isBatteryDir(dir, entry.Name()) {
			continue
		}
		return dir, nil
	}
	return "", errors.New("battery: no battery power supply")
}

func isBatteryDir(dir, name string) bool {
	if _, err := os.Stat(filepath.Join(dir, "capacity")); err != nil {
		return false
	}
	typeData, err := os.ReadFile(filepath.Join(dir, "type"))
	if err == nil && strings.TrimSpace(string(typeData)) == "Battery" {
		return true
	}
	return strings.HasPrefix(name, "BAT")
}
