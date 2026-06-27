//go:build windows

package app

import (
	"fmt"
	"os"
)

func sendReloadSignal() int {
	fmt.Fprintln(os.Stderr, "ptyline: --reload is not supported on Windows")
	return 1
}
