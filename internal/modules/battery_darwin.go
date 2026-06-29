//go:build darwin

package modules

/*
#cgo LDFLAGS: -framework CoreFoundation -framework IOKit
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/ps/IOPowerSources.h>
#include <IOKit/ps/IOPSKeys.h>

typedef struct {
	int found;   // 1 if an internal battery power source was found
	int percent; // 0..100
	int state;   // 0 unknown, 1 charging, 2 discharging, 3 full, 4 not_charging
} batteryInfo;

// readBattery walks the IOKit power sources and returns the first internal
// battery's capacity and charge state. All CoreFoundation handling stays in C so
// the Go side only deals with plain ints; the blob and list are CFRelease'd.
static batteryInfo readBattery() {
	batteryInfo out = {0, 0, 0};

	CFTypeRef blob = IOPSCopyPowerSourcesInfo();
	if (!blob) {
		return out;
	}
	CFArrayRef list = IOPSCopyPowerSourcesList(blob);
	if (!list) {
		CFRelease(blob);
		return out;
	}

	CFIndex n = CFArrayGetCount(list);
	for (CFIndex i = 0; i < n; i++) {
		CFDictionaryRef ps = IOPSGetPowerSourceDescription(blob, CFArrayGetValueAtIndex(list, i));
		if (!ps) {
			continue;
		}

		// Skip UPS / non-battery sources.
		CFStringRef type = CFDictionaryGetValue(ps, CFSTR(kIOPSTypeKey));
		if (type && !CFEqual(type, CFSTR(kIOPSInternalBatteryType))) {
			continue;
		}

		int cur = 0, max = 0;
		CFNumberRef curRef = CFDictionaryGetValue(ps, CFSTR(kIOPSCurrentCapacityKey));
		CFNumberRef maxRef = CFDictionaryGetValue(ps, CFSTR(kIOPSMaxCapacityKey));
		if (curRef) {
			CFNumberGetValue(curRef, kCFNumberIntType, &cur);
		}
		if (maxRef) {
			CFNumberGetValue(maxRef, kCFNumberIntType, &max);
		}
		if (max > 0) {
			out.percent = (cur * 100) / max;
		}

		CFStringRef sourceState = CFDictionaryGetValue(ps, CFSTR(kIOPSPowerSourceStateKey));
		CFBooleanRef charging = CFDictionaryGetValue(ps, CFSTR(kIOPSIsChargingKey));
		CFBooleanRef charged = CFDictionaryGetValue(ps, CFSTR(kIOPSIsChargedKey));
		if (charged && CFBooleanGetValue(charged)) {
			out.state = 3; // full
		} else if (charging && CFBooleanGetValue(charging)) {
			out.state = 1; // charging
		} else if (sourceState && CFEqual(sourceState, CFSTR(kIOPSACPowerValue))) {
			out.state = 4; // on AC, not charging
		} else {
			out.state = 2; // running on battery
		}

		out.found = 1;
		break;
	}

	CFRelease(list);
	CFRelease(blob);
	return out;
}
*/
import "C"

import (
	"context"
	"errors"
)

var errBatteryUnavailable = errors.New("battery provider unavailable")

// darwinBatteryProvider reads battery state from IOKit power sources, the macOS
// counterpart to the Linux sysfs reader.
type darwinBatteryProvider struct{}

func newBatteryProvider() sampler[BatterySample] {
	return darwinBatteryProvider{}
}

func (p darwinBatteryProvider) Probe(ctx context.Context) error {
	_, err := p.Sample(ctx)
	return err
}

func (p darwinBatteryProvider) Sample(ctx context.Context) (BatterySample, error) {
	select {
	case <-ctx.Done():
		return BatterySample{}, ctx.Err()
	default:
	}

	info := C.readBattery()
	if info.found == 0 {
		// No internal battery (desktop Mac): stay hidden, exactly like Linux.
		return BatterySample{}, errBatteryUnavailable
	}

	percent := int(info.percent)
	if percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}
	return BatterySample{Percent: percent, State: darwinBatteryState(int(info.state))}, nil
}

// darwinBatteryState maps readBattery's numeric state to the canonical strings
// shared with the Linux path (see normalizeBatteryState).
func darwinBatteryState(state int) string {
	switch state {
	case 1:
		return "charging"
	case 2:
		return "discharging"
	case 3:
		return "full"
	case 4:
		return "not_charging"
	default:
		return "unknown"
	}
}
