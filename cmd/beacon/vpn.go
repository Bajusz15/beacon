package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"beacon/internal/identity"

	"github.com/spf13/cobra"
)

// createVPNCommand returns the `beacon vpn` command tree.
//
// The model is config-driven: each subcommand edits ~/.beacon/config.yaml and
// the master picks up the change on its next reconcile tick. This mirrors
// `beacon tunnel` and avoids the agent having two separate state machines for
// "what should be running" vs "what is running".
//
// Phase 1 only supports direct connections — at least one device (the exit node)
// must have a reachable UDP port. Phase 2 will add STUN-based hole-punching so
// neither side needs a port forward.
func createVPNCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "vpn",
		Short: "Manage WireGuard VPN between Beacon devices",
		Long: `Beacon VPN — peer-to-peer WireGuard tunnels between your devices.

VPN traffic flows directly between Beacon devices and never transits BeaconInfra
cloud. The cloud is only a key/endpoint coordinator.

Phase 1 requires the exit node to have a reachable UDP port (port-forwarded on
your router or a public IP). The default port is 51820/UDP.

Run "beacon vpn enable" on your home device, then connect from another device.
For laptops/desktops that only need VPN client mode, install the lightweight
"beacon-vpn" binary and run "beacon-vpn connect <home-device>". Full beacon
installs can continue to use "beacon vpn use <home-device>".`,
	}

	enableCmd := &cobra.Command{
		Use:   "enable",
		Short: "Become an exit node — let other Beacon devices route through this one",
		Long: `Mark this device as a VPN exit node. The master agent will:
  1. Generate a WireGuard key pair (~/.beacon/vpn/private.key, encrypted)
  2. Register the public key with BeaconInfra
  3. Bring up the beacon0 TUN interface and enable IP forwarding
  4. Install an iptables NAT rule so client traffic can reach your LAN

The master agent needs root/sudo to create the TUN device — this command
only writes config.`,
		Example: `  beacon vpn enable
  beacon vpn enable --listen-port 51820`,
		Run: runVPNEnable,
	}
	enableCmd.Flags().Int("listen-port", 51820, "UDP port WireGuard listens on (must be port-forwarded on your router)")

	useCmd := &cobra.Command{
		Use:     "use <device-name>",
		Aliases: []string{"connect"},
		Short:   "Connect to another Beacon device's exit node",
		Long: `Mark this device as a VPN client of <device-name>. The master agent will:
  1. Generate a WireGuard key pair if needed
  2. Register with BeaconInfra
  3. Fetch the peer's public key + endpoint
  4. Bring up the beacon0 TUN interface and configure the peer

The peer must have run "beacon vpn enable" first.
The master agent needs root/sudo — this command only writes config.

On client-only laptops/desktops, prefer the standalone beacon-vpn binary:
  beacon-vpn connect my-pi`,
		Example: `  beacon vpn use my-pi
  beacon vpn connect my-pi`,
		Args: cobra.ExactArgs(1),
		Run:  runVPNUse,
	}

	disableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Tear down the VPN and deregister with BeaconInfra",
		Run:   runVPNDisable,
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show VPN status (role, peer, transfer counters)",
		Run:   runVPNStatus,
	}

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Export WireGuard config files (Phase 3 — placeholder)",
	}
	exportCmd := &cobra.Command{
		Use:   "export --device <name>",
		Short: "Generate a wg-quick config + QR code for a phone client (not implemented yet)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("`beacon vpn config export` is a Phase 3 feature and not yet implemented.")
			fmt.Println("For now, use `beacon vpn use` between Beacon-managed devices.")
		},
	}
	exportCmd.Flags().String("device", "", "Device name to generate config for")
	configCmd.AddCommand(exportCmd)

	root.AddCommand(enableCmd, useCmd, disableCmd, statusCmd, configCmd)
	return root
}

func runVPNEnable(cmd *cobra.Command, args []string) {
	listenPort, _ := cmd.Flags().GetInt("listen-port")
	if listenPort <= 0 {
		listenPort = 51820
	}

	// SetVPNExitNode persists the intent. The actual interface comes up when the
	// master reconciles — we don't bring up WireGuard from the CLI process because
	// it would die when the shell exits.
	if err := identity.SetVPNExitNode(listenPort, ""); err != nil {
		logger.Fatalf("beacon vpn enable: %v", err)
	}
	fmt.Printf("Marked as VPN exit node (listen port %d).\n", listenPort)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Forward UDP port %d on your router to this device.\n", listenPort)
	fmt.Println("  2. Make sure beacon start is running with NET_ADMIN capability.")
	fmt.Println("     Grant capabilities (one-time, re-run after each binary update):")
	fmt.Println("       sudo setcap cap_net_admin,cap_net_raw+eip $(which beacon)")
	fmt.Println("     Then run without sudo:")
	fmt.Println("       beacon start --foreground")
	deviceName := ""
	if uc, err := identity.LoadUserConfig(); err == nil && uc != nil && uc.DeviceName != "" {
		deviceName = uc.DeviceName
	}
	if deviceName != "" {
		fmt.Printf("  3. On another Beacon device, run: beacon vpn use %s\n", deviceName)
	} else {
		fmt.Println("  3. On another Beacon device, run: beacon vpn use <this-device-name>")
	}
	fmt.Println()
	fmt.Println("Security note: WireGuard is cryptographically silent — port scanners can't")
	fmt.Println("tell the forwarded port from a closed one without your private key.")
}

func runVPNUse(cmd *cobra.Command, args []string) {
	peer := args[0]
	if err := identity.SetVPNClient(peer, ""); err != nil {
		logger.Fatalf("beacon vpn use: %v", err)
	}
	fmt.Printf("Marked as VPN client of %q.\n", peer)
	fmt.Println("The master agent will fetch the peer's key + endpoint and bring up the tunnel on its next reconcile tick.")
	fmt.Println("Run `beacon vpn status` to monitor the connection.")
}

func runVPNDisable(cmd *cobra.Command, args []string) {
	if err := identity.ClearVPN(); err != nil {
		logger.Fatalf("beacon vpn disable: %v", err)
	}
	fmt.Println("VPN disabled. The master will tear down the tunnel and deregister with BeaconInfra.")
}

func runVPNStatus(cmd *cobra.Command, args []string) {
	cfg, err := identity.LoadUserConfig()
	if err != nil {
		logger.Fatalf("beacon vpn status: %v", err)
	}
	if cfg == nil || cfg.VPN == nil || !cfg.VPN.Enabled {
		fmt.Println("VPN is not enabled on this device.")
		fmt.Println("Run `beacon vpn enable` (exit node) or `beacon vpn use <peer>` (client).")
		return
	}

	// Pull live state from the master's local /api/status — same pattern as `beacon tunnel list`.
	live := fetchVPNLiveStatus()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()
	_, _ = fmt.Fprintf(w, "Role:\t%s\n", cfg.VPN.Role)
	if cfg.VPN.PeerDevice != "" {
		_, _ = fmt.Fprintf(w, "Peer:\t%s\n", cfg.VPN.PeerDevice)
	}
	if cfg.VPN.VPNAddress != "" {
		_, _ = fmt.Fprintf(w, "VPN Address:\t%s\n", cfg.VPN.VPNAddress)
	}
	if cfg.VPN.ListenPort > 0 {
		_, _ = fmt.Fprintf(w, "Listen Port:\t%d\n", cfg.VPN.ListenPort)
	}
	if live != nil {
		connected := "no"
		if live.Connected {
			connected = "yes"
		}
		_, _ = fmt.Fprintf(w, "Connected:\t%s\n", connected)
		if !live.LastHandshake.IsZero() {
			_, _ = fmt.Fprintf(w, "Last handshake:\t%s ago\n", time.Since(live.LastHandshake).Round(time.Second))
		}
		_, _ = fmt.Fprintf(w, "Bytes RX:\t%s\n", humanBytes(live.BytesRx))
		_, _ = fmt.Fprintf(w, "Bytes TX:\t%s\n", humanBytes(live.BytesTx))
		if live.PeerEndpoint != "" {
			_, _ = fmt.Fprintf(w, "Peer endpoint:\t%s\n", live.PeerEndpoint)
		}
		if live.Error != "" {
			_, _ = fmt.Fprintf(w, "Error:\t%s\n", live.Error)
		}
	} else {
		_, _ = fmt.Fprintln(w, "Live state:\tunavailable (is `beacon start` running?)")
	}
}

// vpnLiveStatus mirrors the JSON shape the master serves at /api/status under "vpn".
type vpnLiveStatus struct {
	Enabled       bool      `json:"enabled"`
	Role          string    `json:"role"`
	VPNAddress    string    `json:"vpn_address"`
	PeerDevice    string    `json:"peer_device"`
	PeerEndpoint  string    `json:"peer_endpoint"`
	Connected     bool      `json:"connected"`
	LastHandshake time.Time `json:"last_handshake"`
	BytesRx       uint64    `json:"bytes_rx"`
	BytesTx       uint64    `json:"bytes_tx"`
	Error         string    `json:"error"`
}

func fetchVPNLiveStatus() *vpnLiveStatus {
	cfg, _ := identity.LoadUserConfig()
	port := 9100
	if cfg != nil && cfg.MetricsPort > 0 {
		port = cfg.MetricsPort
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/api/status")
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	var snap struct {
		VPN *vpnLiveStatus `json:"vpn"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil
	}
	return snap.VPN
}

func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
