// Package vpn implements the WireGuard-based peer-to-peer VPN between Beacon devices.
//
// Architecture: BeaconInfra is only a key/endpoint coordinator. VPN traffic never
// transits the cloud — it flows directly between Beacon devices. Phase 1 requires
// at least one device (the "exit node") to have a reachable UDP port (port-forwarded
// or public IP). Phase 2 adds STUN-based NAT traversal so two NATted devices can
// hole-punch a direct path.
package vpn

import "time"

// Role identifies a device's purpose in a VPN topology.
type Role string

const (
	// RoleExitNode means this device exposes its LAN / acts as the gateway.
	RoleExitNode Role = "exit_node"
	// RoleClient means this device routes traffic out through a peer exit node.
	RoleClient Role = "client"
)

// PeerInfo is everything an agent needs to configure a WireGuard peer.
// Returned by GET /api/agent/vpn/peer.
type PeerInfo struct {
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
	Endpoint   string `json:"endpoint"`     // host:port — may be empty if peer hasn't reported one
	VPNAddress string `json:"vpn_address"`  // 10.13.37.x assigned by server
	AllowedIPs string `json:"allowed_ips"`  // CIDR(s) routed to this peer
}

// Status is the runtime state of the local WireGuard interface.
// Surfaced via the master /api/status endpoint and `beacon vpn status`.
type Status struct {
	Enabled       bool      `json:"enabled"`
	Role          Role      `json:"role"`
	InterfaceName string    `json:"interface_name,omitempty"`
	VPNAddress    string    `json:"vpn_address,omitempty"`
	ListenPort    int       `json:"listen_port,omitempty"`
	PublicKey     string    `json:"public_key,omitempty"`
	PeerDevice    string    `json:"peer_device,omitempty"`
	PeerEndpoint  string    `json:"peer_endpoint,omitempty"`
	Connected     bool      `json:"connected"`
	LastHandshake time.Time `json:"last_handshake,omitempty"`
	BytesRx       uint64    `json:"bytes_rx"`
	BytesTx       uint64    `json:"bytes_tx"`
	Error         string    `json:"error,omitempty"`
}

// DefaultListenPort is WireGuard's IANA-registered UDP port. Beacon defaults to it
// because it's what users will most often see in port-forwarding tutorials.
const DefaultListenPort = 51820

// InterfaceName is the TUN device name Beacon creates. Kept stable so users can
// reason about iptables / routing rules.
const InterfaceName = "beacon0"
