package modules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

var errMemoryUnavailable = errors.New("memory provider unavailable")

// MemorySample is one host memory reading in bytes.
type MemorySample struct {
	Total     uint64
	Available uint64
	Used      uint64
	Percent   float64
}

// NewMemory builds the {memory} system module (host memory usage).
func NewMemory(interval time.Duration, format string) status.ProbeModule {
	return newSystemModule("memory", interval, format, "mem {percent}%", newMemoryProvider(), formatMemory)
}

func parseMeminfo(data string) (MemorySample, error) {
	values := map[string]uint64{}
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return MemorySample{}, fmt.Errorf("parse meminfo %s: %w", key, err)
		}
		multiplier := uint64(1)
		if len(fields) >= 3 && fields[2] == "kB" {
			multiplier = 1024
		}
		values[key] = value * multiplier
	}

	total := values["MemTotal"]
	available := values["MemAvailable"]
	if total == 0 {
		return MemorySample{}, errors.New("parse meminfo: missing MemTotal")
	}
	if available == 0 {
		return MemorySample{}, errors.New("parse meminfo: missing MemAvailable")
	}
	if available > total {
		available = total
	}
	used := total - available
	return MemorySample{
		Total:     total,
		Available: available,
		Used:      used,
		Percent:   100 * float64(used) / float64(total),
	}, nil
}

func formatMemory(sample MemorySample, format string) string {
	replacer := strings.NewReplacer(
		"{percent}", fmt.Sprintf("%.0f", sample.Percent),
		"{used_mb}", strconv.FormatUint(sample.Used/1024/1024, 10),
		"{total_mb}", strconv.FormatUint(sample.Total/1024/1024, 10),
		"{available_mb}", strconv.FormatUint(sample.Available/1024/1024, 10),
	)
	return replacer.Replace(format)
}
