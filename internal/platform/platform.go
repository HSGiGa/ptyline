// Package platform contains build-tagged, OS-specific environment classification.
// Exactly one of linux.go / darwin.go / windows.go compiles per target. The
// result is a raw Verdict that runtimeenv normalizes — keep OS-name logic here,
// not scattered through the codebase (spec §4.1).
package platform

// Verdict is the raw, OS-specific classification produced by Detect.
type Verdict struct {
	IsLinux   bool
	IsMacOS   bool
	IsWindows bool
	IsWSL     bool // only meaningful when IsLinux is true
}
