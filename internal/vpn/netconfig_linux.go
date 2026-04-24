//go:build linux

package vpn

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// configureInterfaceLinux assigns the VPN address to the TUN device and brings the
// link up. We shell out to `ip` rather than netlink for two reasons: it works
// without CAP_NET_ADMIN-aware bindings, and it produces shell-readable errors
// homelab users can paste into bug reports.
func configureInterfaceLinux(ifName, vpnAddress string) error {
	if !commandExists("ip") {
		return fmt.Errorf("`ip` command not found — install iproute2")
	}
	// /24 because every Beacon device gets an address inside the user's 10.13.37.0/24.
	if err := runCmd("ip", "addr", "add", vpnAddress+"/24", "dev", ifName); err != nil {
		// "File exists" — address already assigned, ignore
		if !strings.Contains(err.Error(), "File exists") {
			return fmt.Errorf("ip addr add: %w", err)
		}
	}
	if err := runCmd("ip", "link", "set", "dev", ifName, "up"); err != nil {
		return fmt.Errorf("ip link up: %w", err)
	}
	return nil
}

// enableIPForwardingLinux flips net.ipv4.ip_forward = 1. Required on the exit node
// so the kernel will route packets between beacon0 and the LAN egress interface.
// We don't make this persistent (sysctl.conf) because Beacon shouldn't silently
// modify boot-time defaults; users who want it permanent can do so themselves.
func enableIPForwardingLinux() error {
	const path = "/proc/sys/net/ipv4/ip_forward"
	return os.WriteFile(path, []byte("1\n"), 0644)
}

// setupNATMasqueradeLinux installs an iptables MASQUERADE rule so traffic coming
// in from the WireGuard subnet gets NATed out the egress interface. Without this
// the exit node would forward packets but they'd be dropped by the upstream router
// (their source IP would still be 10.13.37.x).
//
// We use -C first to avoid duplicate rules on master restarts.
func setupNATMasqueradeLinux(egressIface string) error {
	if !commandExists("iptables") {
		return fmt.Errorf("`iptables` not found — required for VPN NAT")
	}
	args := []string{"-t", "nat", "-A", "POSTROUTING", "-s", "10.13.37.0/24", "-o", egressIface, "-j", "MASQUERADE"}
	checkArgs := []string{"-t", "nat", "-C", "POSTROUTING", "-s", "10.13.37.0/24", "-o", egressIface, "-j", "MASQUERADE"}
	if err := runCmd("iptables", checkArgs...); err == nil {
		return nil // already installed
	}
	if err := runCmd("iptables", args...); err != nil {
		return fmt.Errorf("iptables masquerade: %w", err)
	}
	return nil
}

// teardownNATMasqueradeLinux removes the MASQUERADE rule installed above.
// Errors are ignored — best-effort cleanup.
func teardownNATMasqueradeLinux(egressIface string) {
	args := []string{"-t", "nat", "-D", "POSTROUTING", "-s", "10.13.37.0/24", "-o", egressIface, "-j", "MASQUERADE"}
	_ = runCmd("iptables", args...)
}

// detectDefaultEgressLinux returns the name of the interface that holds the
// default route — the one MASQUERADE should send traffic out of.
func detectDefaultEgressLinux() (string, error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", fmt.Errorf("ip route show default: %w", err)
	}
	// "default via 192.168.1.1 dev eth0 ..."
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}
	return "", fmt.Errorf("could not detect default egress interface from: %s", string(out))
}

// applyExitNodeNetworkLinux is called on `vpn enable`: brings the interface up,
// enables forwarding, and sets up MASQUERADE on the detected egress.
func applyExitNodeNetwork(ifName, vpnAddress string) (string, error) {
	if err := configureInterfaceLinux(ifName, vpnAddress); err != nil {
		return "", err
	}
	if err := enableIPForwardingLinux(); err != nil {
		return "", fmt.Errorf("enable ip_forward: %w", err)
	}
	egress, err := detectDefaultEgressLinux()
	if err != nil {
		return "", err
	}
	if err := setupNATMasqueradeLinux(egress); err != nil {
		return egress, err
	}
	return egress, nil
}

// applyClientNetwork is called on `vpn use <peer>`: assigns the address only —
// no NAT, no forwarding. Routing of "send all traffic via beacon0" is opt-in
// (Phase 1 ships with /32 AllowedIPs so by default only peer-to-peer traffic
// uses the tunnel).
func applyClientNetwork(ifName, vpnAddress string) error {
	return configureInterfaceLinux(ifName, vpnAddress)
}

// teardownNetwork removes anything applyExitNodeNetwork installed. Idempotent.
func teardownNetwork(ifName, egressIface string) {
	if egressIface != "" {
		teardownNATMasqueradeLinux(egressIface)
	}
	_ = runCmd("ip", "link", "set", "dev", ifName, "down")
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
