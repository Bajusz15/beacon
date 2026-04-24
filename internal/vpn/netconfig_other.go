//go:build !linux && !darwin

package vpn

import "errors"

// Stub implementations for unsupported platforms — Beacon currently builds for
// Linux and macOS. The CLI surfaces the error from these stubs so users on
// other OSes get an immediate, clear failure rather than a panic.

func applyExitNodeNetwork(ifName, vpnAddress string) (string, error) {
	return "", errors.New("VPN is only supported on Linux and macOS")
}

func applyClientNetwork(ifName, vpnAddress string) error {
	return errors.New("VPN is only supported on Linux and macOS")
}

func teardownNetwork(ifName, egressIface string) {}
