//go:build windows

package daemon

import (
	"os"
	"syscall"
)

// sendSignal sends a signal to a process on Windows
// Note: Windows doesn't support Unix signals, we use TerminateProcess for SIGTERM
func sendSignal(pid int, sig syscall.Signal) error {
	// Windows doesn't have signals, so we find and terminate the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	// On Windows, Signal is a no-op for most signals except Kill
	return process.Kill()
}

// signalProcessCheck checks if process exists on Windows
func signalProcessCheck(pid int) error {
	// Try to open the process to check if it exists
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return err
	}
	syscall.CloseHandle(handle)
	return nil
}
