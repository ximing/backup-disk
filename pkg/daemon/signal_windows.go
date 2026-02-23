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

// SignalProcessCheck exports signalProcessCheck for use by other packages
func SignalProcessCheck(pid int) error {
	return signalProcessCheck(pid)
}

// SendSignal exports sendSignal for use by other packages
func SendSignal(pid int, sig syscall.Signal) error {
	return sendSignal(pid, sig)
}

// IsProcessRunning checks if a process with the given PID is running
func IsProcessRunning(pid int) bool {
	return signalProcessCheck(pid) == nil
}
