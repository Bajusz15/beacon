# Beacon

<img src="./docs/logo.png" alt="Beacon Logo" width="120" height="120">

**Local-first monitoring and deployment for self-hosted devices.**

Beacon is a lightweight agent for your Raspberry Pi, N100 mini PC, or any Linux/macOS machine. It gives you a local dashboard with CPU, RAM, disk, temperature, and per-project health — no account, no cloud, no internet required. Optionally connect to [BeaconInfra](https://beaconinfra.dev) for a hosted multi-device view.

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

# Start the master agent
beacon master
```

Open **http://localhost:9100** — that's your local dashboard. No config needed, works fully offline.

> **Tip:** `beacon init` writes `~/.beacon/config.yaml` where you can pin a device name, metrics port, and project list. It's optional — the master uses sensible defaults without it.

---

## What You Get

### Terminal — `beacon status`

Connects to the running master and shows a colored summary:

```
⬡ beacon v0.3.1-beta  ● master running  pid 1847  uptime 14d 3h

DEVICE  pi-homelab  192.168.1.42  arm64  Debian 12

SYSTEM  cpu 12% ████░░░░░░░░░░░░  mem 67% ██████████░░░░░░  disk 41% ██████░░░░░░░░░░
        load 0.42 0.38 0.35  temp 48°C

CHILDREN  3 healthy  1 warning  0 down

  ● portfolio-site      v2.1.0   deployed 2h ago    3/3 checks passing
  ● home-assistant      v2024.3  deployed 5d ago    2/2 checks passing
  ◐ nextcloud           v28.0.1  deployed 3d ago    2/3 checks passing
    └─ ⚠ HTTP https://cloud.local/status  timeout 5.2s > 3s threshold
```

Flags: `--json`, `--watch` (refresh every 5s), `--no-color`, `--port <N>`.

### Browser — `http://localhost:9100`

Self-contained HTML dashboard served by the master. No CDN, no external dependencies. Auto-refreshes every 10s.

- `/api/status` — JSON API
- `/metrics` — Prometheus format
- `/health` — simple health check

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
└─────────────────┘   └───────────────────┘
```

The master is **stateless per project** — it doesn't know about Docker or systemd. Children are isolated: one crash doesn't affect others. The master auto-restarts failed children with exponential backoff.

---

## Set Up a Project

Beacon manages your apps end-to-end: clone from Git or pull from Docker registries, run your deploy command, poll for updates, health check, and tail logs. Each project gets its own isolated child agent.

### Interactive setup

```bash
beacon bootstrap myapp
```

The wizard asks for your deployment type (Git or Docker), repo URL, tokens, and deploy command. It creates a systemd service and **kicks off the first deployment** — then returns you to the terminal.

If systemd isn't available (containers, macOS, minimal installs), bootstrap still creates all the config files — just run `beacon deploy` yourself.

### From a config file

```bash
beacon bootstrap myapp -f bootstrap.yml
```

**Git deployment:**

```yaml
# bootstrap.yml
deployment_type: "git"
repo_url: "https://github.com/you/myapp.git"
git_token: "ghp_xxxxxxxxxxxx"           # or use ssh_key_path for SSH
local_path: "$HOME/beacon/myapp"
deploy_command: "./scripts/deploy.sh"
poll_interval: "60s"
port: "8080"
secure_env_path: "/etc/beacon/myapp.env"
```

**Docker deployment:**

```yaml
# bootstrap.yml
deployment_type: "docker"
local_path: "$HOME/beacon/myapp"
poll_interval: "60s"
docker_images:
  - image: "ghcr.io/you/web-app"
    token: "ghp_xxxxxxxxxxxx"
    deploy_command: "docker compose up -d"
    docker_compose_files:
      - "docker-compose.yml"
  - image: "docker.io/you/api-server"
    deploy_command: "docker stop api && docker rm api && docker run -d --name api ${BEACON_DOCKER_IMAGE}"
```

Supports Docker Hub, GitHub Container Registry (ghcr.io), and any Docker Registry v2-compatible registry. Each image is tracked independently — if `web-app` gets a new tag but `api-server` doesn't, only `web-app` redeploys.

See [examples/](./examples/) for more bootstrap configs (multi-image, private registries, compose overrides).

### What bootstrap creates

- `~/.beacon/config/projects/myapp/env` — deploy environment (tokens, paths, commands)
- `~/.beacon/config/projects/myapp/monitor.yml` — health check config
- `beacon@myapp.service` — systemd service that runs `beacon deploy` (skipped if systemd unavailable)
- Appends the project to `~/.beacon/config.yaml` so the master spawns a child for it

### How deploy works

`beacon deploy` is a long-running process that polls for new releases and deploys automatically. Bootstrap sets it up as a systemd service, but you can also run it directly.

**Git mode:** every `poll_interval`, runs `git fetch --tags`, detects the newest tag, clones it, and runs your `deploy_command`. Auth supports HTTPS tokens (GitHub, GitLab) and SSH keys.

**Docker mode:** every `poll_interval`, queries the registry API for the latest image tag, runs `docker pull`, then runs your `deploy_command` with these environment variables:

| Variable | Example |
|----------|---------|
| `BEACON_DOCKER_IMAGE` | `ghcr.io/you/app:v1.2.0` |
| `BEACON_DOCKER_TAG` | `v1.2.0` |
| `BEACON_DOCKER_COMPOSE_FILES` | `docker-compose.yml docker-compose.prod.yml` |

### Health checks

Add checks to the monitor config:

```yaml
# ~/.beacon/config/projects/myapp/monitor.yml
checks:
  - name: "http_200"
    type: http
    url: "http://localhost:8080/health"
    interval: 30s

  - name: "process_running"
    type: process
    name: "myapp"
```

Wire the project into the master via `~/.beacon/config.yaml`:

```yaml
projects:
  - id: "myapp"
    config_path: "/home/user/.beacon/config/projects/myapp/monitor.yml"
```

Restart the master and the project appears in `beacon status` and the dashboard.

### Application secrets

`secure_env_path` in your bootstrap config points to **your application's** environment file — where you keep `DATABASE_URL`, `API_SECRET`, etc. Beacon loads it at deploy time so your app has its secrets. This file is separate from Beacon's own config.

---

## Cloud Dashboard (Optional)

Everything above works without an internet connection. The cloud is purely additive — connect to [BeaconInfra](https://beaconinfra.dev) to view all your devices from anywhere.

**Without an API key, Beacon makes zero network requests.**

```bash
# Save credentials and enable cloud reporting
beacon cloud login --api-key usr_abc123def456

# Restart master to begin heartbeats
beacon master
```

The first successful heartbeat registers the device — there's no separate registration step.

To disable: `beacon cloud logout` clears the key and stops all cloud reporting.

For self-hosted backends: `beacon cloud login --cloud-url https://your-host.example.com/api`

### `~/.beacon/config.yaml`

Created by `beacon init` (local-only) or `beacon cloud login` (with API key). You can also edit it directly.

```yaml
api_key: "usr_abc123def456"       # set by beacon cloud login (omit for offline)
device_name: "my-pi"              # defaults to hostname
cloud_url: "https://beaconinfra.dev/api"
heartbeat_interval: 30            # seconds
cloud_reporting_enabled: true
metrics_port: 9100                # local dashboard port

projects:
  - id: "myapp"
    config_path: "/home/user/.beacon/config/projects/myapp/monitor.yml"
  - id: "blog"
    config_path: "/home/user/.beacon/config/projects/blog/monitor.yml"
    enabled: false                # temporarily disabled
```

---

## Where Files Live

All state lives under **`~/.beacon`** (override with `BEACON_HOME`):

```
~/.beacon/
  config.yaml                    # Master config + project list
  config/projects/<id>/env       # Per-project deploy environment
  config/projects/<id>/monitor.yml
  state/                         # Check results, deploy status
  ipc/                           # Master <-> child communication
  keys/                          # Encrypted token store (beacon keys)
  templates/                     # Alert templates
  logs/
```

Note: `~/beacon` (no dot) is the default **deploy working tree** for cloned repos, not the agent config root.

Inspect paths: `beacon config show`

---

## Commands

| Command | Purpose |
|---------|---------|
| `beacon master` | Start master agent (dashboard at :9100, child agents, optional heartbeats) |
| `beacon status` | Terminal health view from running master (`--json`, `--watch`, `--no-color`) |
| `beacon init` | Write local `config.yaml` (`--name`, `--metrics-port`; no network) |
| `beacon cloud login` / `logout` | Enable/disable cloud reporting |
| `beacon config show` | Print resolved paths, identity, and project count |
| `beacon bootstrap <name>` | Set up a new project (interactive or `-f config.yml`) |
| `beacon deploy` | Git/Docker tag polling loop (also the default with no subcommand) |
| `beacon monitor [-f config.yml]` | Run one project's health checks (debug) |
| `beacon projects list\|add\|remove\|status\|info` | Project inventory management |
| `beacon keys list\|add\|rotate\|delete\|validate` | Encrypted local token store |
| `beacon alerts init\|status\|test\|acknowledge\|resolve` | Alert routing |
| `beacon template list\|add\|remove\|show\|init` | Alert templates (Discord, Slack, email, webhook) |
| `beacon setup-wizard` | Interactive monitor YAML + env helper |
| `beacon source add\|list\|remove\|status` | Observation sources (e.g. Kubernetes) |
| `beacon mcp serve` | MCP server for Cursor / Claude Desktop |
| `beacon version` | Version info |

Hidden: `beacon agent` (child process, spawned by master only).

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `BEACON_HOME` | Override data directory (default: `~/.beacon`) |
| `BEACON_API_KEY` | API key for `beacon cloud login` when not passed as a flag |
| `BEACON_DEVICE_NAME` | Device name when `--name` is omitted |
| `NO_COLOR` | Disable ANSI colors in `beacon status` |

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

- [docs/MASTER_AGENT.md](./docs/MASTER_AGENT.md) — master/child architecture and heartbeats
- [docs/LOG_FORWARDING.md](./docs/LOG_FORWARDING.md) — log forwarding configuration
- [docs/KEY_MANAGEMENT.md](./docs/KEY_MANAGEMENT.md) — encrypted key store
- [docs/MCP.md](./docs/MCP.md) — MCP server for editors
- [docs/E2E_TESTING.md](./docs/E2E_TESTING.md) — E2E test setup
- [beacon.monitor.yml](./beacon.monitor.yml) — monitoring config reference
- [examples/](./examples/) — bootstrap, monitor, alert, and wizard examples

---

## License

Apache 2.0 — see [LICENSE](./LICENSE)
