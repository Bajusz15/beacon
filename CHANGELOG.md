# Changelog

All notable changes to Beacon are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added
- **Beacon VPN (WireGuard)** — peer-to-peer encrypted tunnel between Beacon devices.
  BeaconInfra acts only as a key/endpoint coordinator; VPN traffic never transits the cloud.
  - `beacon vpn enable` — configure device as exit node
  - `beacon vpn use <device>` — connect to an exit node as client
  - `beacon vpn disable` — tear down tunnel and deregister
  - `beacon vpn status` — show connection state, peer, tx/rx bytes
  - Private keys stored AES-GCM encrypted under `~/.beacon/vpn/private.key`
  - VPN status reported in heartbeat and visible on local dashboard
  - Phase 1: direct connections (port-forwarded exit node). Phase 2 (STUN hole-punching) planned.
- **VPN piggyback commands** — `vpn_enable`, `vpn_use`, `vpn_disable` can be triggered
  remotely from BeaconInfra dashboard via heartbeat response. Agent writes config and the
  master's Reconcile loop brings the tunnel up/down.
- **Command dispatcher hardening** — defense-in-depth for heartbeat piggyback commands:
  - Action allowlist — only known actions accepted (`restart`, `stop`, `health_check`,
    `fetch_logs`, `tunnel_connect`, `vpn_enable`, `vpn_use`, `vpn_disable`). Unknown actions
    rejected with a warning.
  - Command deduplication — tracks seen command IDs (1h TTL). Replay of the same command
    is silently skipped.
  - `allowed_remote_commands` config option — user can override the default allowlist in
    `~/.beacon/config.yaml` to restrict which actions the device accepts remotely.
- **`beacon update`** — self-update command. Fetches latest release from GitHub, verifies
  SHA256 checksum, and atomically replaces the binary. Use `--check` to only check.
- **`beacon projects redeploy <name>`** — pull latest code and re-run the deploy command
  for a project. Reads the project location from the inventory and `BEACON_DEPLOY_CMD`
  from the project's env file.
- **Auto-init on `beacon start`** — if no `~/.beacon/config.yaml` exists, the master
  automatically runs the equivalent of `beacon init` (using system hostname) instead of
  starting with a nil config.
- **VPN security docs** (`docs/VPN.md`) — full handshake flow diagram showing how
  BeaconInfra coordinates Curve25519 key exchange, step-by-step setup guide (N100 exit
  node + laptop on phone hotspot), WireGuard-vs-exposed-app comparison table, security
  model summary, CLI reference, troubleshooting.
- VPN test suite — 40+ new tests across 4 packages:
  - `cloud/vpn_test.go` — httptest server for register/getpeer/deregister wire format
  - `vpn/keys_test.go` — encryption roundtrip, file permissions, corrupted-blob rejection
  - `vpn/manager_test.go` — Reconcile state machine with mock PeerResolver
  - `identity/userconfig_test.go` — VPN config helpers (SetVPNExitNode/Client, ClearVPN)
  - `master/dispatcher_test.go` — allowlist, dedup, VPN command dispatch

### Changed
- Release workflow now creates GitHub Releases with binaries attached (was artifact-only).
  VERSION uses git tag instead of date-based string.
- VPN manager `shutdownLocked` no longer shells out to `ip`/`ifconfig` when no device
  was ever created (avoids unnecessary syscalls on masters that never enable VPN).
- `beacon vpn enable` now prints the actual device name instead of `<this-device-name>`.
- `beacon vpn enable` no longer suggests `sudo beacon start` — recommends `setcap` instead
  to avoid spawning child processes as root.
- `beacon update` creates temp file in the same directory as the binary, fixing
  cross-filesystem rename failures when the binary is in `/usr/local/bin`.

### Removed
- **Kubernetes observer** (`internal/k8sobserver/`) — entire package removed (~1,400 lines).
  Included informers, workload tracking, digest computation, storage layer, CLI, and
  integration tests. Removed to reduce binary size and narrow Beacon's focus to the
  core homelab/self-hosted use case. Kubernetes monitoring may return as a separate plugin.
- **Template engine** (`internal/templates/`) — engine, manager, CLI removed (~600 lines).
  Was used for generating Kubernetes manifests and config files from templates.
- **Kubernetes dependencies** — `k8s.io/client-go`, `k8s.io/api`, `k8s.io/apimachinery`
  and all transitive dependencies removed from `go.mod`. Reduced binary size by ~8MB.
- Kubernetes RBAC example (`examples/kubernetes/rbac.yaml`).

## [0.3.1-beta] - 2025-12-15

### Added
- Secure tunneling with cloud relay (WebSocket-based).
- `beacon tunnel list` / `beacon tunnel enable` / `beacon tunnel disable` CLI.
- Structured logger (`internal/logging`) replacing raw `log.Printf`.
- MCP server (`beacon mcp serve`) for Cursor/Claude Desktop integration.
- System metrics in heartbeat (CPU, memory, disk, load average).
- Piggyback commands from heartbeat response (restart, stop, health_check, fetch_logs, tunnel_connect).
- Local dashboard fix for Home Assistant status display.

### Changed
- Tunneling rewritten from scratch (new approach with proper reconnection).
