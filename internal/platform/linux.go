//go:build linux

package platform

// Detect classifies a Linux host, distinguishing native Linux from WSL2.
func Detect() Verdict {
	return Verdict{IsLinux: true, IsWSL: isWSL()}
}
