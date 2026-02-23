//go:build !windows

package daemon

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// sendSignal sends a signal to a process
func sendSignal(pid int, sig syscall.Signal) error {
	return unix.Kill(pid, unix.Signal(sig))
}

// signalProcessCheck sends signal 0 to check if process exists
func signalProcessCheck(pid int) error {
	return unix.Kill(pid, 0)
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
