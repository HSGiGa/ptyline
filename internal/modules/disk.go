package modules

import (
	"strconv"
	"strings"
	"time"

	"github.com/hsgiga/ptyline/internal/status"
)

// DiskSample is one filesystem usage reading in bytes.
type DiskSample struct {
	Total   uint64
	Free    uint64
	Used    uint64
	Percent float64
}

// NewDisk builds the {disk} system module: filesystem usage for the shell cwd,
// falling back to the platform root. cwd may be nil.
func NewDisk(interval time.Duration, format string, cwd func() string) status.ProbeModule {
	return newSystemModule("disk", interval, format, "disk {percent}%", newDiskProvider(cwd), formatDisk)
}

// diskPath resolves the filesystem path to stat: the shell cwd when available,
// else "/".
func diskPath(cwd func() string) string {
	if cwd != nil {
		if dir := strings.TrimSpace(cwd()); dir != "" {
			return dir
		}
	}
	return "/"
}

func diskSample(total, free uint64) DiskSample {
	if free > total {
		free = total
	}
	used := total - free
	var percent float64
	if total > 0 {
		percent = 100 * float64(used) / float64(total)
	}
	return DiskSample{Total: total, Free: free, Used: used, Percent: percent}
}

func formatDisk(sample DiskSample, format string) string {
	replacer := strings.NewReplacer(
		"{percent}", formatPercent(sample.Percent),
		"{used_gb}", strconv.FormatUint(sample.Used/1024/1024/1024, 10),
		"{free_gb}", strconv.FormatUint(sample.Free/1024/1024/1024, 10),
		"{total_gb}", strconv.FormatUint(sample.Total/1024/1024/1024, 10),
	)
	return replacer.Replace(format)
}
