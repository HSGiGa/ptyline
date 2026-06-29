package modules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

var errBatteryUnavailable = errors.New("battery provider unavailable")

// BatterySample is one battery reading.
type BatterySample struct {
	Percent int
	State   string
}

// NewBattery builds the {battery} system module (percentage + charging state).
func NewBattery(interval time.Duration, format string) status.ProbeModule {
	return newSystemModule("battery", interval, format, "bat {percent}%", newBatteryProvider(), formatBattery)
}

func parseBatteryCapacity(data string) (int, error) {
	percent, err := strconv.Atoi(strings.TrimSpace(data))
	if err != nil {
		return 0, fmt.Errorf("parse battery capacity: %w", err)
	}
	if percent < 0 {
		return 0, nil
	}
	if percent > 100 {
		return 100, nil
	}
	return percent, nil
}

func normalizeBatteryState(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "charging":
		return "charging"
	case "discharging":
		return "discharging"
	case "full":
		return "full"
	case "not charging":
		return "not_charging"
	default:
		return "unknown"
	}
}

func formatBattery(sample BatterySample, format string) string {
	replacer := strings.NewReplacer(
		"{percent}", strconv.Itoa(sample.Percent),
		"{state}", sample.State,
	)
	return replacer.Replace(format)
}
