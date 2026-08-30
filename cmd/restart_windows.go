//go:build windows

package cmd

import (
	"os"
	"os/exec"
)

// Windows has no exec-replace; spawn a detached successor and exit.
func restartProcess(exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
