package modules

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

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
		"{percent}", formatPercent(float64(sample.Percent)),
		"{state}", sample.State,
		"{icon}", batteryIcon(sample),
	)
	return replacer.Replace(format)
}

// Material Design (Nerd Font) battery glyphs, indexed by charge bucket (nearest
// 10%). Nerd Fonts maps these at the original MDI codepoints in the U+F0xxx
// range. The {icon} placeholder is a Nerd-Font convenience for the battery
// format; ASCII users simply omit it.
var batteryGlyphs = map[int]string{
	0:   "\U000F0083", // mdi-battery-alert (empty)
	10:  "\U000F007A", // mdi-battery-10
	20:  "\U000F007B", // mdi-battery-20
	30:  "\U000F007C", // mdi-battery-30
	40:  "\U000F007D", // mdi-battery-40
	50:  "\U000F007E", // mdi-battery-50
	60:  "\U000F007F", // mdi-battery-60
	70:  "\U000F0080", // mdi-battery-70
	80:  "\U000F0081", // mdi-battery-80
	90:  "\U000F0082", // mdi-battery-90
	100: "\U000F0079", // mdi-battery (full)
}

var batteryChargingGlyphs = map[int]string{
	0:   "\U000F089C", // mdi-battery-charging-10 (no charging-0 glyph)
	10:  "\U000F089C", // mdi-battery-charging-10
	20:  "\U000F0086", // mdi-battery-charging-20
	30:  "\U000F0087", // mdi-battery-charging-30
	40:  "\U000F0088", // mdi-battery-charging-40
	50:  "\U000F089D", // mdi-battery-charging-50
	60:  "\U000F0089", // mdi-battery-charging-60
	70:  "\U000F089E", // mdi-battery-charging-70
	80:  "\U000F008A", // mdi-battery-charging-80
	90:  "\U000F008B", // mdi-battery-charging-90
	100: "\U000F0085", // mdi-battery-charging-100
}

// batteryIcon picks a battery glyph from the charge level and charging state.
// Charging shows the bolt variant; a charged ("full") battery on AC shows the
// full glyph; everything else uses the plain level ramp.
func batteryIcon(sample BatterySample) string {
	bucket := (sample.Percent + 5) / 10 * 10
	if bucket < 0 {
		bucket = 0
	} else if bucket > 100 {
		bucket = 100
	}
	switch sample.State {
	case "charging":
		return batteryChargingGlyphs[bucket]
	case "full":
		return batteryGlyphs[100]
	default:
		return batteryGlyphs[bucket]
	}
}
