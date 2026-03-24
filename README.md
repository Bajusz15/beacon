# Beacon

<img src="./docs/logo.png" alt="Beacon Logo" width="120" height="120">

**Lightweight agent for self-hosted devices — deploy, monitor, and report health from a single binary.**

Beacon runs on your Raspberry Pi, N100 mini PC, or any Linux box. A **master agent** runs in the background managing independent **child agents** per project. Each child handles its own health checks, log tailing, and deployments. The master aggregates everything and optionally reports to the [BeaconInfra](https://beaconinfra.dev) cloud dashboard — or you run it fully offline.

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-lightgrey)](https://github.com/Bajusz15/beacon/releases)
[![CI](https://github.com/Bajusz15/beacon/workflows/CI/badge.svg)](https://github.com/Bajusz15/beacon/actions)
[![codecov](https://codecov.io/gh/Bajusz15/beacon/branch/main/graph/badge.svg)](https://codecov.io/gh/Bajusz15/beacon)
[![Go Report Card](https://goreportcard.com/badge/github.com/Bajusz15/beacon)](https://goreportcard.com/report/github.com/Bajusz15/beacon)

---

## How Beacon Works

```
┌──────────────────────────────────────────────┐
│            BeaconInfra Cloud (optional)       │
│   receives heartbeats, sends commands back    │
└───────────────────┬──────────────────────────┘
                    │ HTTPS  (optional)
                    │
┌───────────────────┴──────────────────────────┐
│             beacon master                     │
│                                               │
│  - Runs in the background (one per device)    │
│  - Collects device metrics: CPU, RAM, disk    │
│  - Sends heartbeat to cloud on interval       │
│  - Aggregates health from all child agents    │
│  - Dispatches commands to children via IPC    │
│  - Spawns, watches, and restarts children     │
└──────┬───────────────────────┬───────────────┘
       │  IPC (file-based)     │  IPC
       │                       │
┌──────┴──────────┐   ┌────────┴──────────┐
│  child agent    │   │  child agent      │  ... N children
│  project: myapp │   │  project: blog    │
│                 │   │                   │
│  - Health checks│   │  - Health checks  │
│  - Log tailing  │   │  - Log tailing    │
│  - Deployments  │   │  - Deployments    │
└─────────────────┘   └───────────────────┘
```

**One binary. Two modes:**

- `beacon master` — the background daemon that manages everything
- `beacon agent` — internal child mode, spawned by the master (never run directly)

The master is **stateless per project** — it doesn't know about Docker or systemd. Each child agent handles its own project config and reports health back up through IPC. Adding a new project type never requires changing the master.

**Children are isolated.** One crash doesn't affect others. The master auto-restarts failed children with exponential backoff.

---

## Privacy First

Cloud reporting is **completely optional**. By default, if you don't run `beacon init`, the master runs with `cloud_reporting_enabled: false` and never makes a network request.

When enabled, Beacon sends device metrics (CPU %, RAM %, disk %, uptime) and application health summaries. It uses your machine's hostname as the device identifier — no fingerprinting, no telemetry beyond what you configure.

**Application-level metrics** (e.g. HTTP response time, process CPU) are opt-in per project in the project's `beacon.monitor.yml`. You control exactly what gets reported.

---

## Quick Start

### 1. Install

```bash
curl -fsSL https://raw.githubusercontent.com/Bajusz15/beacon/main/scripts/install.sh | bash
```

### 2. Bootstrap your first project

```bash
# Interactive setup (recommended)
beacon bootstrap myapp

# Or from a config file
beacon bootstrap myapp -f beacon.bootstrap.example.yml
```

Bootstrap creates a project config at `~/.beacon/config/projects/myapp/`, sets up a systemd service, and optionally configures cloud reporting.

### 3. Start the master agent

```bash
beacon master
```

The master spawns child agents for each configured project, starts health checks, and (if configured) sends heartbeats to the cloud.

For production, run it as a systemd service — `beacon bootstrap` installs this automatically.

---

## Cloud Reporting (Optional)

To connect to [BeaconInfra](https://beaconinfra.dev) (or a self-hosted compatible backend):

### 1. Write local config — no network call

```bash
beacon init --api-key usr_xxxx --name my-pi --cloud-url https://api.beaconinfra.dev/api
```

This writes `~/.beacon/config.yaml`. No HTTP requests are made here. The device is registered on the **first heartbeat** — there is no separate register step.

### 2. Start the master

```bash
beacon master
```

The first heartbeat creates the device in the dashboard using `(user, device_name)` as the identity. The server returns a `device_id` which is cached locally for convenience — it's not authoritative.

### Config file: `~/.beacon/config.yaml`

```yaml
api_key: "usr_xxxxxxxxxxxxxxxx"
device_name: "my-pi"              # defaults to hostname if omitted
cloud_url: "https://api.beaconinfra.dev/api"
heartbeat_interval: 30            # seconds
cloud_reporting_enabled: true

# Projects managed by this device's master agent
projects:
  - id: "myapp"
    config_path: "/home/user/myapp/beacon.monitor.yml"
    enabled: true
```

### Environment variables

```bash
BEACON_API_KEY       # user API key
BEACON_CLOUD_URL     # API base URL
BEACON_DEVICE_NAME   # device name (defaults to hostname)
```

---

## Project Setup

Each project has its own `beacon.monitor.yml`:

```yaml
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

# Optional: application-specific metrics to include in heartbeat
metrics:
  http_response_ms: true    # adds response time to heartbeat payload
  process_cpu_percent: true
```

All metrics fields are opt-in. Leave the `metrics:` section out entirely to report only status (healthy/degraded/down).

---

## Commands

```bash
# Setup
beacon init [--api-key <key>] [--name <name>] [--cloud-url <url>]
beacon bootstrap <project> [-f config.yml]

# Run
beacon master                   # start master + all child agents (foreground)
beacon monitor [-f config.yml]  # run a single child agent directly (dev/debug)

# Manage
beacon projects list            # list configured projects
beacon projects status          # health summary
beacon restart master           # systemctl --user restart beacon-master.service

# Other
beacon version
beacon --help
```

---

## Systemd (Production)

`beacon bootstrap` installs and starts systemd user services automatically.

For manual setup:

```bash
# Master agent
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

- [docs/MASTER_AGENT.md](./docs/MASTER_AGENT.md) — master agent architecture and IPC contract
- [docs/LOG_FORWARDING.md](./docs/LOG_FORWARDING.md) — log forwarding configuration
- [beacon.monitor.example.yml](./beacon.monitor.yml) — monitoring config reference
- [examples/](./examples/) — bootstrap and config examples

---

## License

Apache 2.0 — see [LICENSE](./LICENSE)
