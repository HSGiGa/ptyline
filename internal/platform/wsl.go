//go:build linux

package platform

import (
	"os"
	"strings"
)

// isWSL reports whether the Linux host is running under WSL/WSL2. WSL is a
// runtime branch of the Linux binary, not a separate build target (spec §4.1).
//
// Detection uses the kernel release string, which contains "microsoft" / "WSL"
// under WSL. TODO scaffold (plan 01): also honor $WSL_DISTRO_NAME and probe
// /proc/sys/kernel/osrelease robustly.
func isWSL() bool {
	if _, ok := os.LookupEnv("WSL_DISTRO_NAME"); ok {
		return true
	}
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	s := strings.ToLower(string(data))
	return strings.Contains(s, "microsoft") || strings.Contains(s, "wsl")
}
