//go:build linux

package daemon

import (
	"fmt"
	"os"
	"syscall"
)

// daemonizeImpl implements daemonization for Linux
func (d *Daemon) daemonizeImpl() error {
	// Fork the process
	pid, _, errno := syscall.Syscall(syscall.SYS_FORK, 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("fork failed: %v", errno)
	}

	if pid > 0 {
		// Parent process - exit
		os.Exit(0)
	}

	// Child process continues as daemon

	// Create new session
	_, err := syscall.Setsid()
	if err != nil {
		return fmt.Errorf("setsid failed: %w", err)
	}

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

	syscall.Dup2(int(devNull.Fd()), int(os.Stdin.Fd()))
	syscall.Dup2(int(devNull.Fd()), int(os.Stdout.Fd()))
	syscall.Dup2(int(devNull.Fd()), int(os.Stderr.Fd()))

	return nil
}
