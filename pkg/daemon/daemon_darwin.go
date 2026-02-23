//go:build darwin

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// daemonizeImpl implements daemonization for macOS
func (d *Daemon) daemonizeImpl() error {
	// On macOS, we use ForkExec instead of raw fork
	// Fork a new process
	procAttr := &syscall.ProcAttr{
		Dir:   "/",
		Env:   os.Environ(),
		Files: []uintptr{uintptr(syscall.Stdin), uintptr(syscall.Stdout), uintptr(syscall.Stderr)},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	}

	// Get the executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Fork and exec
	pid, err := syscall.ForkExec(execPath, os.Args, procAttr)
	if err != nil {
		return fmt.Errorf("fork/exec failed: %w", err)
	}

	if pid > 0 {
		// Parent process - exit
		os.Exit(0)
	}

	// Child process continues as daemon

	// Redirect standard file descriptors to /dev/null
	devNull, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open /dev/null: %w", err)
	}
	defer devNull.Close()

	syscall.Dup2(int(devNull.Fd()), syscall.Stdin)
	syscall.Dup2(int(devNull.Fd()), syscall.Stdout)
	syscall.Dup2(int(devNull.Fd()), syscall.Stderr)

	return nil
}

// reexecDaemon re-executes the current process as a daemon
// This is an alternative approach used on macOS
func (d *Daemon) reexecDaemon() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon process: %w", err)
	}

	// Parent exits
	os.Exit(0)
	return nil // Never reached
}
