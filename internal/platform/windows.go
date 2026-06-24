//go:build windows

package platform

// Detect classifies a Windows host (ConPTY backend).
func Detect() Verdict {
	return Verdict{IsWindows: true}
}
