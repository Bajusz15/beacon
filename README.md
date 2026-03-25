# Beacon

<img src="./docs/logo.png" alt="Beacon Logo" width="120" height="120">

**Local-first monitoring for self-hosted devices.**

Beacon is a lightweight agent for your Raspberry Pi, N100 mini PC, or any Linux box. Install it, run `beacon master`, and get a local dashboard with CPU, RAM, disk, temperature, and project health. Everything runs on your device — no account, no cloud, no internet required.

Optionally connect to [BeaconInfra](https://beaconinfra.dev) to view all your devices from anywhere.

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-lightgrey)](https://github.com/Bajusz15/beacon/releases)
[![CI](https://github.com/Bajusz15/beacon/workflows/CI/badge.svg)](https://github.com/Bajusz15/beacon/actions)
[![codecov](https://codecov.io/gh/Bajusz15/beacon/branch/main/graph/badge.svg)](https://codecov.io/gh/Bajusz15/beacon)
[![Go Report Card](https://goreportcard.com/badge/github.com/Bajusz15/beacon)](https://goreportcard.com/report/github.com/Bajusz15/beacon)

---

## Quick Start

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/Bajusz15/beacon/main/scripts/install.sh | bash

# Run
beacon master
```

Open **http://localhost:9100** — you'll see your device metrics in real-time. That's it.

Without any config file, Beacon runs fully offline. **No API key = no network requests. Ever.**

---

## What You Get

### Terminal — `beacon status`

```
⬡ beacon v0.3.1-beta  ● master running  pid 1847  uptime 14d 3h

DEVICE  pi-homelab  192.168.1.42  arm64  Debian 12

SYSTEM  cpu 12% ████░░░░░░░░░░░░  mem 67% ██████████░░░░░░  disk 41% ██████░░░░░░░░░░
        load 0.42 0.38 0.35  temp 48°C

CHILDREN  3 healthy  1 warning  0 down

  ● portfolio-site      v2.1.0   deployed 2h ago    3/3 checks passing
  ● home-assistant      v2024.3  deployed 5d ago    2/2 checks passing
  ● pi-hole             v5.18    deployed 12d ago   4/4 checks passing
  ◐ nextcloud           v28.0.1  deployed 3d ago    2/3 checks passing
    └─ ⚠ HTTP https://cloud.local/status  timeout 5.2s > 3s threshold

RECENT  last 24h
  14:32  deploy    portfolio-site v2.1.0 → success (12s)
  11:07  alert     nextcloud HTTP check timeout (3 consecutive)
  09:00  heartbeat cloud sync OK (metrics + logs)

metrics http://localhost:9100  prometheus http://localhost:9100/metrics
```

### Browser — `http://localhost:9100`

Self-contained HTML dashboard served by the master. No CDN, no external dependencies. Auto-refreshes every 10 seconds.

- Real-time system metrics (CPU, memory, disk, load, temperature)
- Child agent status with health check details
- Recent events timeline (deploys, alerts, restarts, syncs)
- Prometheus metrics at `/metrics`
- JSON API at `/api/status`

### Architecture

```
┌──────────────────────────────────────────────┐
│          BeaconInfra Cloud (optional)         │
└───────────────────┬──────────────────────────┘
                    │ HTTPS
┌───────────────────┴──────────────────────────┐
│             beacon master                     │
│                                               │
│  One per device. Collects system metrics,     │
│  serves local dashboard, sends heartbeats.    │
│  Spawns a child agent per project.            │
└──────┬───────────────────────┬───────────────┘
       │  IPC (file-based)     │
┌──────┴──────────┐   ┌────────┴──────────┐
│  child agent    │   │  child agent      │  ...
│  project: myapp │   │  project: blog    │
│  health checks  │   │  health checks    │
│  log tailing    │   │  log tailing      │
│  deployments    │   │  deployments      │
└─────────────────┘   └───────────────────┘
```

The master is **stateless per project** — it doesn't know about Docker or systemd. Children are isolated: one crash doesn't affect others. The master auto-restarts failed children with exponential backoff.

---

## Monitor Your Apps

Beacon can watch your applications. Each project gets its own child agent with health checks, log tailing, and deployments.

```bash
beacon bootstrap myapp
```

Or write a config manually:

```yaml
# ~/myapp/beacon.monitor.yml
device:
  name: "my-pi"

checks:
  - name: "http_200"
    type: http
    url: "http://localhost:8080/health"
    interval: 30s

  - name: "process_running"
    type: process
    name: "myapp"
```

Add it to `~/.beacon/config.yaml`:

```yaml
projects:
  - id: "myapp"
    config_path: "/home/user/myapp/beacon.monitor.yml"
```

Restart the master and the project appears in `beacon status` and the dashboard.

---

## Cloud Dashboard (Optional)

Beacon is local-first. Everything above works without an internet connection and without any account. The cloud is purely additive — if you want to view your devices from a phone or another machine, you can connect to [BeaconInfra](https://beaconinfra.dev).

**Without an API key, Beacon makes zero network requests.**

To enable cloud reporting, create an API key at **beaconinfra.dev → Settings → API Keys** and run:

```bash
beacon init --api-key usr_abc123def456
beacon master
```

The cloud URL defaults to `https://beaconinfra.dev/api`. Override it with `--cloud-url` or the `BEACON_CLOUD_URL` environment variable for self-hosted backends or testing.

### `~/.beacon/config.yaml`

This file is created by `beacon init`. You can also edit it directly.

```yaml
api_key: "usr_abc123def456"       # from beaconinfra.dev
device_name: "my-pi"              # defaults to hostname
cloud_url: "https://beaconinfra.dev/api"
heartbeat_interval: 30            # seconds
cloud_reporting_enabled: true
metrics_port: 9100                # local dashboard port

projects:
  - id: "myapp"
    config_path: "/home/user/myapp/beacon.monitor.yml"
  - id: "blog"
    config_path: "/home/user/blog/beacon.monitor.yml"
    enabled: false                # temporarily disabled
```

### Environment variables

| Variable | Description |
|----------|-------------|
| `BEACON_API_KEY` | User API key (alternative to `--api-key`) |
| `BEACON_CLOUD_URL` | API base URL (alternative to `--cloud-url`) |
| `BEACON_DEVICE_NAME` | Device name (alternative to `--name`) |
| `NO_COLOR` | Disable ANSI colors in `beacon status` |

---

## Commands

```bash
beacon master                    # start the agent (dashboard at :9100)
beacon status                    # device + project health in terminal
beacon status --json             # JSON output for scripting
beacon status --watch            # auto-refresh every 5s
beacon status --no-color         # plain text (respects NO_COLOR env var)

beacon init --api-key usr_...    # save cloud config locally (no network)
beacon bootstrap <project>       # interactive project setup

beacon monitor [-f config.yml]   # run a single project monitor (dev/debug)
beacon deploy                    # poll Git for new tags and deploy

beacon version                   # show version info
beacon mcp serve                 # MCP server for Cursor / Claude Desktop
```

---

## Run as a Service

`beacon bootstrap` installs systemd services automatically. For manual setup:

```bash
cat > ~/.config/systemd/user/beacon-master.service << 'EOF'
[Unit]
Description=Beacon Master Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/beacon master
Restart=on-failure
RestartSec=30

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now beacon-master.service
```

---

## CI & Quality

| Check | Description |
|-------|-------------|
| Tests | Go 1.24, all platforms |
| Platforms | Linux ARM / ARM64 / AMD64, macOS ARM64 / AMD64 |
| Security | gosec + govulncheck |
| Lint | golangci-lint |
| Coverage | [codecov](https://codecov.io/gh/Bajusz15/beacon) |

---

## Documentation

- [docs/MASTER_AGENT.md](./docs/MASTER_AGENT.md) — master/child architecture and IPC contract
- [docs/LOG_FORWARDING.md](./docs/LOG_FORWARDING.md) — log forwarding configuration
- [beacon.monitor.yml](./beacon.monitor.yml) — monitoring config reference
- [examples/](./examples/) — bootstrap and config examples

---

## License

Apache 2.0 — see [LICENSE](./LICENSE)
