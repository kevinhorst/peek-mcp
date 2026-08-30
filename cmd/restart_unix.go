//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

func restartProcess(exe string) error {
	return syscall.Exec(exe, os.Args, os.Environ())
}
