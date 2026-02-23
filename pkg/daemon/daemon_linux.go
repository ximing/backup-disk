//go:build linux

package daemon

// signalParentSuccess is a no-op on Linux
// The daemon now runs in foreground mode only, managed by systemd or other process managers
func (d *Daemon) signalParentSuccess() {
	// No-op: daemon runs in foreground mode
}
