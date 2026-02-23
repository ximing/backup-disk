//go:build darwin

package daemon

// signalParentSuccess is a no-op on Darwin
// The daemon now runs in foreground mode only, managed by launchd or other process managers
func (d *Daemon) signalParentSuccess() {
	// No-op: daemon runs in foreground mode
}
