//go:build linux

package daemon

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// daemonizeImpl implements daemonization for Linux
func (d *Daemon) daemonizeImpl() error {
	// Get executable path (must use absolute path for ForkExec)
	// Try /proc/self/exe first (most reliable on Linux), fallback to os.Executable()
	execPath, err := os.Readlink("/proc/self/exe")
	if err != nil {
		execPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("failed to get executable path: %w", err)
		}
	}

	// Verify the executable exists
	if _, err := os.Stat(execPath); err != nil {
		return fmt.Errorf("executable not found at %s: %w", execPath, err)
	}

	// Get error file path from environment (set by parent)
	errFilePath := os.Getenv("CLOUDSYNC_STARTUP_ERR_FILE")
	pidFilePath := d.pidFile

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
		// Parent process - wait for child to start successfully
		// Child should write PID file on success or error file on failure
		maxWait := 10 // seconds
		for i := 0; i < maxWait*10; i++ {
			time.Sleep(100 * time.Millisecond)

			// Check if error file was written
			if errFilePath != "" {
				if errMsg := readStartupError(errFilePath); errMsg != "" {
					return fmt.Errorf("daemon failed to start: %s", errMsg)
				}
			}

			// Check if PID file was written with child's PID
			if pidData, err := os.ReadFile(pidFilePath); err == nil {
				var childPID int
				if _, err := fmt.Sscanf(string(pidData), "%d", &childPID); err == nil {
					if childPID == pid {
						// Success! Child started and wrote its PID
						fmt.Printf("Daemon started successfully (PID: %d)\n", pid)
						os.Exit(0)
					}
				}
			}

			// Check if child is still running
			if !isProcessRunning(pid) {
				// Child exited without writing error or PID
				return fmt.Errorf("daemon process exited unexpectedly")
			}
		}

		// Timeout waiting for child
		return fmt.Errorf("timeout waiting for daemon to start")
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
