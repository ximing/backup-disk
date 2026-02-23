//go:build windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// daemonizeImpl implements daemonization for Windows
// Note: Windows doesn't support traditional Unix daemonization
// We use a detached process instead
func (d *Daemon) daemonizeImpl() error {
	// On Windows, we start a new detached process
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Dir = ""

	// Hide the console window
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon process: %w", err)
	}

	// Parent exits
	os.Exit(0)
	return nil // Never reached
}
