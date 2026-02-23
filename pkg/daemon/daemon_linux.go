//go:build linux

package daemon

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// daemonizeImpl implements daemonization for Linux
func (d *Daemon) daemonizeImpl() error {
	// Get executable path (must use absolute path for ForkExec)
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Use syscall.ForkExec for proper daemonization
	sysProcAttr := &syscall.SysProcAttr{
		Setsid: true,
	}

	procAttr := &syscall.ProcAttr{
		Dir:   "/",
		Env:   os.Environ(),
		Files: []uintptr{0, 1, 2},
		Sys:   sysProcAttr,
	}

	// Fork and exec
	pid, err := syscall.ForkExec(execPath, os.Args, procAttr)
	if err != nil {
		return fmt.Errorf("fork failed: %w", err)
	}

	if pid > 0 {
		// Parent process - exit
		os.Exit(0)
	}

	// Child process continues as daemon

	// Change working directory to root
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("chdir failed: %w", err)
	}

	// Redirect standard file descriptors to /dev/null
	devNull, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open /dev/null: %w", err)
	}
	defer devNull.Close()

	unix.Dup2(int(devNull.Fd()), unix.Stdin)
	unix.Dup2(int(devNull.Fd()), unix.Stdout)
	unix.Dup2(int(devNull.Fd()), unix.Stderr)

	return nil
}
