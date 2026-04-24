//go:build darwin

package vpn

import (
	"fmt"
	"os/exec"
	"strings"
)

// macOS support is intentionally minimal — Beacon's primary deployment target is
// Linux (Raspberry Pi, N100 mini PCs). The macOS path exists so developers can
// smoke-test the agent on their laptops. Full exit-node behaviour (NAT + IP
// forwarding via pfctl) is not implemented; client mode works for connecting
// out to a Linux exit node.

// configureInterfaceDarwin assigns the VPN address to the utun device. macOS's
// userspace WireGuard creates utunN devices, not the requested name — the
// caller passes the actual interface name returned by tun.Device.Name().
func configureInterfaceDarwin(ifName, vpnAddress string) error {
	if !commandExists("ifconfig") {
		return fmt.Errorf("ifconfig not found")
	}
	// `ifconfig utun4 10.13.37.5 10.13.37.5 netmask 255.255.255.0 up`
	if err := runCmd("ifconfig", ifName, "inet", vpnAddress, vpnAddress, "netmask", "255.255.255.0", "up"); err != nil {
		return fmt.Errorf("ifconfig: %w", err)
	}
	return nil
}

func applyExitNodeNetwork(ifName, vpnAddress string) (string, error) {
	if err := configureInterfaceDarwin(ifName, vpnAddress); err != nil {
		return "", err
	}
	// Note: enabling IP forwarding + pfctl NAT on macOS requires modifying /etc/pf.conf
	// and reloading pf. We don't do that automatically — too invasive for a smoke-test
	// platform. Document the manual steps in docs/VPN.md.
	return "", fmt.Errorf("macOS exit-node mode is not fully supported in Phase 1 — run an exit node on Linux instead")
}

func applyClientNetwork(ifName, vpnAddress string) error {
	return configureInterfaceDarwin(ifName, vpnAddress)
}

func teardownNetwork(ifName, egressIface string) {
	_ = runCmd("ifconfig", ifName, "down")
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
