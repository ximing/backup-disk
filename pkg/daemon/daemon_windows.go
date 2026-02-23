//go:build windows

package daemon

// signalParentSuccess is a no-op on Windows
// The daemon now runs in foreground mode only, managed by Windows Service or other process managers
func (d *Daemon) signalParentSuccess() {
	// No-op: daemon runs in foreground mode
}
