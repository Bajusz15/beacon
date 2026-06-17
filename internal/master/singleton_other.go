//go:build !unix

package master

// On non-unix platforms advisory flock isn't available; the beacon daemon targets
// Linux/macOS (both "unix"). These stubs keep the package building elsewhere with no
// single-instance guard (lock is a no-op, and the soft pid check reports "not running").
func lockExclusiveNB(int) error { return nil }

func processAlive(int) bool { return false }
