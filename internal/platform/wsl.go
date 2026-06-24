//go:build linux

package platform

import (
	"os"
	"strings"
)

// isWSL reports whether the Linux host is running under WSL/WSL2. WSL is a
// runtime branch of the Linux binary, not a separate build target (spec §4.1).
func isWSL() bool {
	return isWSLFrom(os.LookupEnv, os.ReadFile)
}

func isWSLFrom(
	lookupEnv func(string) (string, bool),
	readFile func(string) ([]byte, error),
) bool {
	if _, ok := lookupEnv("WSL_DISTRO_NAME"); ok {
		return true
	}
	data, err := readFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	s := strings.ToLower(string(data))
	return strings.Contains(s, "microsoft") || strings.Contains(s, "wsl")
}
