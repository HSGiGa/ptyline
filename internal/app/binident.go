package app

import (
	"fmt"
	"os"
	"time"
)

// binaryIdentity records the identity of the running binary at startup.
// It is used to detect whether a new binary has been installed so --reload
// can re-exec in place instead of just refreshing config.
type binaryIdentity struct {
	path    string
	modTime time.Time
	size    int64
	ok      bool // false when os.Executable or os.Stat failed at capture time
}

// captureBinaryIdentity records the path and stat of the running binary.
// Called once at the top of run().
func captureBinaryIdentity() binaryIdentity {
	path, err := os.Executable()
	if err != nil {
		return binaryIdentity{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return binaryIdentity{}
	}
	return binaryIdentity{
		path:    path,
		modTime: info.ModTime(),
		size:    info.Size(),
		ok:      true,
	}
}

// changed reports whether the binary on disk differs from the snapshot taken
// at startup. Returns (false, nil) when re-exec is unavailable or the binary
// is unchanged. Returns (false, error) when the binary exists but is not safe
// to exec (not executable, zero-size, or can't be stat'd).
func (b binaryIdentity) changed() (bool, error) {
	if !b.ok {
		return false, nil
	}
	info, err := os.Stat(b.path)
	if err != nil {
		return false, fmt.Errorf("binary %s: %w", b.path, err)
	}
	if info.Size() == 0 {
		return false, fmt.Errorf("binary at %s is empty, skipping re-exec", b.path)
	}
	if info.Mode()&0o111 == 0 {
		return false, fmt.Errorf("binary at %s is not executable, skipping re-exec", b.path)
	}
	return info.ModTime() != b.modTime || info.Size() != b.size, nil
}
