# Beacon VPN

Beacon VPN is a peer-to-peer WireGuard tunnel between your Beacon-managed devices.
Use it to reach your home services from anywhere — Plex, Home Assistant, Jellyfin,
the NAS web UI — without exposing them to the internet.

> **VPN traffic never transits BeaconInfra cloud.** The cloud is *only* a key and
> endpoint coordinator. Traffic flows directly between your devices, peer-to-peer,
> end-to-end encrypted with WireGuard. We physically cannot see your traffic.

---

## Why Beacon VPN is safer than exposing services directly

If you've been thinking about port-forwarding Plex or Home Assistant straight to
the internet, here's the comparison.

| | **Exposed app (Plex/HA/etc.)** | **Beacon VPN (WireGuard)** |
|---|---|---|
| **Port scanner sees…** | Open port + app banner ("Plex 1.40", "Home Assistant 2024.3") | Nothing. Identical to a closed port. |
| **Responds to handshake without auth?** | Yes — TLS handshake, redirect, login page | No. Drops the packet silently. |
| **Discoverable on Shodan?** | Yes — searchable by app name and version | No. Cryptographically silent. |
| **Surface area** | Whole HTTP stack: parsers, deserialization, plugins, auth bugs | One UDP socket → one Curve25519 handshake |
| **Day-zero exposure** | A new Plex/HA CVE = your box is on the menu | A new WireGuard CVE has happened zero times in the protocol's history |
| **Brute-force window** | Login form (rate limit if you're lucky) | None — packets without valid crypto are dropped before any code runs |
| **Logs from random scanners** | Constant noise from /admin, /wp-login, /.env probes | Zero — scanners get no response, so they stop trying |

The "WireGuard is silent" property is the headline:

> WireGuard servers do not respond to any packets that are not authenticated by a
> valid peer. There is no banner, no version string, no error message, no TCP RST.
> A port scanner sees the forwarded UDP port as **identical** to a closed port.

This is by design. It's why WireGuard ports basically never show up on Shodan
unless the operator misconfigures something.

---

## How it works

```
┌────────────────┐                          ┌────────────────┐
│  Your laptop   │                          │  Home N100     │
│  (client)      │◄─── WireGuard tunnel ───►│  (exit node)   │
│                │     direct, encrypted    │                │
└───────┬────────┘                          └────────┬───────┘
        │                                            │
        │  HTTPS heartbeat (key+endpoint exchange)   │
        │                                            │
        └────────────► BeaconInfra cloud ◄───────────┘
                       (no VPN traffic)
```

1. Each device generates a WireGuard key pair locally. **Private keys never
   leave the device** — they're encrypted at rest with the existing master key
   under `~/.beacon/vpn/private.key`.
2. Each device sends only its **public key + listen endpoint** to BeaconInfra
   over the regular heartbeat channel.
3. When you run `beacon vpn use my-pi`, your laptop fetches `my-pi`'s public key
   and endpoint from BeaconInfra and configures a WireGuard peer with it.
4. WireGuard then establishes a direct UDP path between the two devices. The
   handshake and all subsequent traffic flows peer-to-peer.

BeaconInfra never has the private keys, never sees the traffic, and can't
decrypt anything even if compelled to. The cloud is a phone book, not a relay.

---

## Phase 1 — Direct connections (shipping now)

Phase 1 requires the **exit node** (the home device) to have a reachable UDP port:

- A port forward on your router (most common)
- A public IP (for VPS / colo)
- UPnP if your router supports it

The default port is `51820/UDP`. You can change it with `--listen-port`.

This works for ~70% of self-hosters out of the box. Remember: forwarding the
WireGuard port is dramatically safer than forwarding Plex's port (see table
above).

### Setup

**On your home device** (Raspberry Pi, N100, NAS, etc.):

```bash
# 1. Run beacon master at least once so the device is registered.
sudo beacon master --foreground   # or via systemd

# 2. Mark this device as the exit node.
sudo beacon vpn enable
```

Forward UDP `51820` on your router → home device. Done.

**On your laptop / phone / second device**:

```bash
# Same prerequisite — beacon master must be running.
sudo beacon master --foreground

# Connect to the home exit node by name.
sudo beacon vpn use my-home-pi
```

The master agent on each side will reconcile the new config on its next
heartbeat tick (~30 s). Check status with:

```bash
beacon vpn status
```

### What gets routed

By default Beacon VPN ships **/32 routes** — only the peer device's VPN address
(e.g. `10.13.37.2`) is routed through the tunnel. Everything else uses your
normal internet connection.

This is the "reach my home services" mode. A future flag will add "route
everything through home" mode for full traffic backhaul.

---

## Phase 2 — STUN-based NAT traversal (planned)

If neither device has a port forward, Phase 2 uses STUN to discover each
device's public ip:port and coordinates simultaneous "hole-punch" packets to
open NAT bindings on both routers.

This works for ~90%+ of home networks. The exception is **symmetric NAT**
(common with carrier-grade NAT and some mobile carriers), where the source port
changes per destination — hole-punching is impossible without a relay.

Beacon VPN will not ship a relay. Symmetric-NAT users get a clear error message
pointing to port-forwarding docs. We'd rather be honest about the limit than
silently route your traffic through someone else's server.

---

## Security model summary

| Concern | How Beacon VPN handles it |
|---|---|
| **Cloud sees traffic?** | No. Cloud only stores public keys + endpoints. |
| **Cloud can MITM?** | No. WireGuard handshake authenticates both peers via static keys. A malicious cloud could give you a wrong peer key, but the connection would fail (it wouldn't decrypt your real peer's traffic). |
| **Stolen disk image?** | Private key is AES-GCM encrypted under `~/.beacon/.master_key`. Without that file, the encrypted blob is useless. |
| **Stolen API key?** | Attacker can register a new device under your account, but without your private WireGuard key they cannot impersonate your existing devices. They could, however, *trick a future client* into pairing with their device — protect your API key. |
| **Forwarded port scanned?** | Silent. Port appears closed to scanners. |
| **WireGuard CVEs?** | The userspace Go implementation tracks upstream. WireGuard's protocol has had no protocol-level CVEs to date. |

---

## Requirements

- **Linux** (primary target). macOS works for client mode (developer smoke test).
  Windows is not supported.
- **Root / sudo** — required to create the TUN device and install iptables NAT
  rules. The CLI checks early and bails out with a clear message if not root.
- `iproute2` (`ip` command) and `iptables` on Linux. Both are standard on every
  distro Beacon supports.

---

## Troubleshooting

`beacon vpn status` says "unavailable" — is `beacon master` running? The CLI
reads live state from the master's `/api/status` endpoint. If the master isn't
running, only the static config is shown.

**No handshake after a minute** — check that:
1. UDP `51820` (or your custom port) is forwarded on the exit node's router.
2. The exit node's `beacon master` is running (`systemctl --user status beacon-master`).
3. `sudo iptables -t nat -L POSTROUTING` shows the MASQUERADE rule on the exit node.
4. `ip addr show beacon0` shows the assigned `10.13.37.x` address on both ends.

**"hole-punching timed out"** (Phase 2 only) — your router is doing symmetric
NAT. Either set up a port forward on the exit node, or switch to a different
network on the client side.
