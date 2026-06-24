//go:build darwin

package platform

// Detect classifies a macOS host.
func Detect() Verdict {
	return Verdict{IsMacOS: true}
}
