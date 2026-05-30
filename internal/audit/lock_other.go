//go:build !unix

package audit

// On non-unix platforms we fall back to the in-process mutex only. Beacon
// targets Linux and macOS; this stub exists so the package still compiles
// elsewhere.
func lockFD(int) error { return nil }

func unlockFD(int) error { return nil }
