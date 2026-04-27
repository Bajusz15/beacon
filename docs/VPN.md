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

## How it works — the full handshake

Your devices never talk to each other before setup. BeaconInfra acts as a phone
book: it stores public keys and endpoints so each side knows where to find the
other. Once they have that info, all traffic is direct.

```
        N100 (home WiFi)                 BeaconInfra                 Laptop (cafe / hotspot)
             │                               │                              │
 1.  beacon vpn enable                       │                              │
     ─ generate Curve25519 key pair          │                              │
     ─ POST /vpn/register ─────────────────►│                              │
       { public_key: "N100_PUB",             │                              │
         role: "exit_node",                  │                              │
         listen_port: 51820 }                │                              │
             │◄──────────────────────────────│                              │
             │  { vpn_address: "10.13.37.1" }│                              │
             │                               │                              │
             │                               │   2.  beacon vpn use n100    │
             │                               │◄─────────────────────────────│
             │                               │  POST /vpn/register          │
             │                               │  { public_key: "LAPTOP_PUB", │
             │                               │    role: "client" }          │
             │                               │─────────────────────────────►│
             │                               │  { vpn_address: "10.13.37.2"}│
             │                               │                              │
             │                               │◄─────────────────────────────│
             │                               │  GET /vpn/peer?device_name=n100
             │                               │─────────────────────────────►│
             │                               │  { public_key: "N100_PUB",   │
             │                               │    endpoint: "HOME_IP:51820",│
             │                               │    vpn_address: "10.13.37.1"}│
             │                               │                              │
 3.  WireGuard handshake — direct, peer-to-peer, no cloud                   │
             │◄════════════════════════════════════════════════════════════►│
             │           UDP :51820  ◄──►  UDP :51820                       │
             │           Curve25519 mutual authentication                   │
             │                                                              │
 4.  Tunnel is up — all traffic encrypted, direct between devices           │
             │◄═══════ ping 10.13.37.1 ═══════════════════════════════════►│
             │◄═══════ curl 10.13.37.1:8123 (Home Assistant) ═════════════►│
```

**Step by step:**

1. **N100 registers as exit node.** It generates a Curve25519 key pair (private
   key stored AES-GCM encrypted in `~/.beacon/vpn/private.key`), sends only the
   public key to BeaconInfra, and gets a VPN address (`10.13.37.1`) back.

2. **Laptop registers as client.** Same key generation. Then it asks BeaconInfra:
   "give me n100's public key and endpoint." BeaconInfra returns the N100's
   public key + `YOUR_HOME_PUBLIC_IP:51820`.

3. **Laptop sends a WireGuard handshake** directly to `HOME_IP:51820`. The
   packet travels: hotspot → carrier → internet → your home router → port
   forward → N100. The N100 validates the handshake: "is this from a known
   public key?" Yes — the laptop's key was exchanged through BeaconInfra.

4. **Tunnel is up.** Both sides have authenticated each other via Curve25519.
   All subsequent traffic flows peer-to-peer, encrypted with session keys.
   A persistent keepalive (every 25s) keeps NAT bindings alive.

**Why random scanners can't connect:** WireGuard drops any packet that doesn't
contain a valid Curve25519 handshake from a known peer. No public key in the
peer list = silent drop. No response, no error, no indication the port is even
open.

---

## Step-by-step setup guide

This walkthrough uses an N100 mini PC as the home exit node and a MacBook as the
client. Substitute your own device names.

### Prerequisites

- Both devices have Beacon installed and `beacon cloud login` done (same account)
- Both devices have run `beacon start` at least once (so they're registered)
- You have access to your home router's port forwarding settings

### 0. Grant network capabilities (Linux only, one-time)

The master agent needs `CAP_NET_ADMIN` and `CAP_NET_RAW` to create TUN devices
and set up iptables NAT. **Do not run `sudo beacon start`** — that runs the
entire process tree as root, including all child project agents.

Instead, grant just the capabilities the binary needs:

```bash
sudo setcap cap_net_admin,cap_net_raw+eip $(which beacon)
```

After this, `beacon start` works without sudo. You only need to re-run `setcap`
after updating the binary (`beacon update`).

On macOS this step is not needed — `utun` devices are unprivileged.

### 1. Set up the exit node (N100, at home)

```bash
# Start the master agent (or use systemd for auto-start)
beacon start --foreground
```

In another terminal:

```bash
# Enable VPN exit-node mode
beacon vpn enable

# Verify it wrote the config
cat ~/.beacon/config.yaml
# You should see:
#   vpn:
#     enabled: true
#     role: exit_node
#     listen_port: 51820
```

The master agent will pick up the new config on its next tick (~30s), generate
keys, register with BeaconInfra, and bring up the `beacon0` interface.

```bash
# Check status (wait ~30s after enabling)
beacon vpn status
# Should show:
#   Role:        exit_node
#   VPN Address: 10.13.37.1
#   Listen Port: 51820
#   Connected:   false (no peer yet)
```

### 2. Forward the port on your router

Log into your router and add a port forward:

| Field | Value |
|---|---|
| Protocol | UDP |
| External port | 51820 |
| Internal IP | Your N100's LAN IP (e.g. `192.168.1.50`) |
| Internal port | 51820 |

This is the only port you need to forward. Unlike forwarding Plex (port 32400)
or Home Assistant (port 8123), forwarding the WireGuard port exposes zero attack
surface — see the comparison table above.

### 3. Connect from your laptop (different network)

**Important:** Your laptop must be on a **different network** than the N100.
If both are on the same WiFi, the tunnel works but you're not testing the real
scenario.

The easiest way: **connect your laptop to your phone's hotspot.** This puts it
on the carrier's network — same as being at a cafe.

```bash
# On your laptop (connected to phone hotspot, NOT home WiFi):
beacon start --foreground
```

In another terminal:

```bash
# Connect to the N100 by its device name
beacon vpn use n100

# Wait ~30s for the master to reconcile, then check:
beacon vpn status
# Should show:
#   Role:          client
#   VPN Address:   10.13.37.2
#   Peer Device:   n100
#   Peer Endpoint: <your-home-public-ip>:51820
#   Connected:     true
#   Bytes Rx:      1.2 KB
#   Bytes Tx:      0.8 KB
```

### 4. Verify the tunnel

```bash
# Ping the N100 through the tunnel
ping 10.13.37.1

# Access services running on the N100
curl http://10.13.37.1:8123    # Home Assistant
curl http://10.13.37.1:32400   # Plex
curl http://10.13.37.1:8096    # Jellyfin

# Your regular internet still works (we route only /32, not everything)
curl ifconfig.me   # shows your hotspot IP, NOT your home IP
```

On the N100:
```bash
beacon vpn status
# Bytes Rx/Tx should be increasing
```

### 5. Disconnect

```bash
# On the laptop:
beacon vpn disable

# On the N100 (if you want to tear down the exit node too):
beacon vpn disable
```

---

## Phase 1 — Direct connections (current)

Phase 1 requires the **exit node** (the home device) to have a reachable UDP port:

- A port forward on your router (most common)
- A public IP (for VPS / colo)
- UPnP if your router supports it

The default port is `51820/UDP`. You can change it with `--listen-port`.

This works for ~70% of self-hosters out of the box. Remember: forwarding the
WireGuard port is dramatically safer than forwarding Plex's port (see table
above).

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

## CLI reference

```bash
beacon vpn enable [--listen-port 51820]   # Become exit node
beacon vpn use <device-name>              # Connect to an exit node as client
beacon vpn disable                        # Tear down tunnel, deregister
beacon vpn status                         # Show role, address, peer, tx/rx
```

VPN commands only write config — they don't need root. The master agent acts on
the config and needs `CAP_NET_ADMIN` (see setup step 0).

---

## Requirements

- **Linux** (primary target). macOS works for client mode (developer use).
  Windows is not supported.
- **`CAP_NET_ADMIN` + `CAP_NET_RAW`** on the beacon binary (Linux). Use
  `sudo setcap cap_net_admin,cap_net_raw+eip $(which beacon)` once after
  install. Do **not** run the master with `sudo` — it spawns child processes
  that would all run as root.
- `iproute2` (`ip` command) and `iptables` on Linux. Both are standard on every
  distro Beacon supports.

---

## Troubleshooting

`beacon vpn status` says "unavailable" — is `beacon start` running? The CLI
reads live state from the master's `/api/status` endpoint. If the master isn't
running, only the static config is shown.

`beacon vpn status` says "not enabled" after `beacon vpn enable` — make sure
you're not mixing `sudo` and non-sudo. The VPN commands don't need sudo; if you
ran `sudo beacon vpn enable` it wrote to `/root/.beacon/config.yaml` instead of
your user's `~/.beacon/config.yaml`. Fix: `beacon vpn enable` (no sudo).

**No handshake after a minute** — check that:
1. UDP `51820` (or your custom port) is forwarded on the exit node's router.
2. The exit node's `beacon start` is running.
3. `sudo iptables -t nat -L POSTROUTING` shows the MASQUERADE rule on the exit node.
4. `ip addr show beacon0` shows the assigned `10.13.37.x` address on both ends.

**Both devices on the same WiFi?** The tunnel will still work, but you're not
testing the port-forward path. Connect your laptop to a phone hotspot to
simulate being on a different network.

**"hole-punching timed out"** (Phase 2 only) — your router is doing symmetric
NAT. Either set up a port forward on the exit node, or switch to a different
network on the client side.
