package modules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

var errLoadUnavailable = errors.New("load provider unavailable")

// LoadSample is one host load-average reading.
type LoadSample struct {
	Load1  float64
	Load5  float64
	Load15 float64
}

// NewLoad builds the {load} system module (host load average).
func NewLoad(interval time.Duration, format string) status.ProbeModule {
	return newSystemModule("load", interval, format, "load {load1}", newLoadProvider(), formatLoad)
}

func parseLoadavg(data string) (LoadSample, error) {
	fields := strings.Fields(data)
	if len(fields) < 3 {
		return LoadSample{}, fmt.Errorf("parse loadavg: got %d fields", len(fields))
	}
	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return LoadSample{}, fmt.Errorf("parse load1: %w", err)
	}
	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return LoadSample{}, fmt.Errorf("parse load5: %w", err)
	}
	load15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return LoadSample{}, fmt.Errorf("parse load15: %w", err)
	}
	return LoadSample{Load1: load1, Load5: load5, Load15: load15}, nil
}

func formatLoad(sample LoadSample, format string) string {
	replacer := strings.NewReplacer(
		"{load1}", fmt.Sprintf("%.2f", sample.Load1),
		"{load5}", fmt.Sprintf("%.2f", sample.Load5),
		"{load15}", fmt.Sprintf("%.2f", sample.Load15),
	)
	return replacer.Replace(format)
}
