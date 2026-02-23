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
