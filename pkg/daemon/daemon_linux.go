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
	// Check if we're in the parent or child process by looking for the error file env var
	errFilePath := os.Getenv("CLOUDSYNC_STARTUP_ERR_FILE")

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

	// Create a pipe for parent-child synchronization
	var pipe [2]int
	if err := unix.Pipe(pipe[:]); err != nil {
		return fmt.Errorf("failed to create pipe: %w", err)
	}

	// Use syscall.ForkExec for proper daemonization
	sysProcAttr := &syscall.SysProcAttr{
		Setsid: true,
	}

	procAttr := &syscall.ProcAttr{
		Dir:   "/",
		Env:   os.Environ(),
		Files: []uintptr{0, 1, 2, uintptr(pipe[1])}, // Include write end of pipe
		Sys:   sysProcAttr,
	}

	// Pass pipe FD to child via environment
	os.Setenv("CLOUDSYNC_PIPE_FD", fmt.Sprintf("%d", pipe[1]))

	// Fork and exec
	pid, err := syscall.ForkExec(execPath, os.Args, procAttr)
	if err != nil {
		unix.Close(pipe[0])
		unix.Close(pipe[1])
		return fmt.Errorf("fork failed: %w", err)
	}

	// Close write end in parent
	unix.Close(pipe[1])

	if pid > 0 {
		// Parent process - wait for child to signal success or failure
		// Child will write a byte to the pipe when ready, or close it on error
		buf := make([]byte, 1)

		// Set a read timeout
		done := make(chan struct{})
		var readErr error
		var n int

		go func() {
			n, readErr = unix.Read(pipe[0], buf)
			close(done)
		}()

		// Wait for either the read to complete or timeout
		select {
		case <-done:
			unix.Close(pipe[0])

			if readErr != nil || n == 0 {
				// Child closed pipe without writing = error occurred
				// Check error file for details
				if errFilePath != "" {
					if errMsg := readStartupError(errFilePath); errMsg != "" {
						return fmt.Errorf("daemon failed to start: %s", errMsg)
					}
				}
				return fmt.Errorf("daemon process exited unexpectedly")
			}

			// Child wrote a byte = success
			fmt.Printf("Daemon started successfully (PID: %d)\n", pid)
			return nil

		case <-time.After(10 * time.Second):
			unix.Close(pipe[0])
			// Kill the child process
			unix.Kill(pid, unix.SIGTERM)
			return fmt.Errorf("timeout waiting for daemon to start")
		}
	}

	// Child process continues as daemon
	// Close read end in child
	unix.Close(pipe[0])

	// Get the write end of the pipe
	pipeFD := pipe[1]
	if envFD := os.Getenv("CLOUDSYNC_PIPE_FD"); envFD != "" {
		fmt.Sscanf(envFD, "%d", &pipeFD)
	}

	// Store the pipe FD for later signaling
	d.pipeFD = pipeFD

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
