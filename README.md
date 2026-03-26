# Beacon

<img src="./docs/logo.png" alt="Beacon Logo" width="120" height="120">

**Local-first monitoring for self-hosted devices.**

Beacon is a lightweight agent for your Raspberry Pi, N100 mini PC, or any Linux or macOS machine. Install it, run **`beacon master`**, and get a local dashboard with CPU, RAM, disk, temperature, and per-project health when children are configured. **No account is required** for that: the UI and metrics run on your machine and work without internet. **Cloud is optional** — connect to [BeaconInfra](https://beaconinfra.dev) only if you want a hosted multi-device view (`beacon cloud login`).

Optional: run **`beacon init`** first to write `~/.beacon/config.yaml` (device name, metrics port, project list). You can skip it: the master still starts and uses sensible defaults; init is the place to pin `device_name` and layout before you add projects.

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-lightgrey)](https://github.com/Bajusz15/beacon/releases)
[![CI](https://github.com/Bajusz15/beacon/workflows/CI/badge.svg)](https://github.com/Bajusz15/beacon/actions)
[![codecov](https://codecov.io/gh/Bajusz15/beacon/branch/main/graph/badge.svg)](https://codecov.io/gh/Bajusz15/beacon)
[![Go Report Card](https://goreportcard.com/badge/github.com/Bajusz15/beacon)](https://goreportcard.com/report/github.com/Bajusz15/beacon)

---

## Important: default command vs master

Running **`beacon` with no subcommand starts deploy mode** (long-running Git/Docker tag polling). That is **not** the local dashboard.

| Goal | Command |
|------|---------|
| Local dashboard + child agents | **`beacon master`** |
| Poll repo/registry and deploy | **`beacon deploy`** (or plain `beacon`) |
| Terminal view of master health | **`beacon status`** (master must be running) |

---

## Quick Start (local, no account)

```bash
curl -fsSL https://raw.githubusercontent.com/Bajusz15/beacon/main/scripts/install.sh | bash

# Optional: create ~/.beacon/config.yaml (local only; skip if you only want defaults)
beacon init

beacon master
```

Open **http://localhost:9100**. Cloud heartbeats only run after **`beacon cloud login`** (and can be stopped with **`beacon cloud logout`**).

---

## End-to-end flow (recommended order)

### Phase A — Run the master locally

1. Install the binary (see above).
2. Optionally: `beacon init` to write `~/.beacon/config.yaml` with `device_name` (default: hostname) and `cloud_reporting_enabled: false`.
3. `beacon master` — local dashboard and (when configured) child agents.
4. `beacon status` — colored summary (requires master on port 9100).

### Phase B — Projects (deploy + monitoring)

For each application:

1. **`beacon bootstrap <project>`** (or `beacon bootstrap <project> -f bootstrap.yml`) — creates `~/.beacon/config/projects/<project>/`, deploy env, and **appends** this project to `~/.beacon/config.yaml` `projects:` pointing at `.../monitor.yml`.
2. Start deploy: e.g. `systemctl --user start beacon@<project>` or run `beacon deploy` with that project’s env.
3. Copy or edit **`~/.beacon/config/projects/<project>/monitor.yml`** (see [beacon.monitor.yml](./beacon.monitor.yml)) — health checks, alerts, etc.
4. Restart **`beacon master`** so children pick up the monitor config.

`beacon monitor -f <path>` is useful to **debug one project’s checks** without the master; production flow is master + children.

### Phase C — BeaconInfra (optional, after local setup)

1. Create an account and API key at [beaconinfra.dev](https://beaconinfra.dev) → Settings → API Keys.
2. `beacon cloud login` (interactive), or `beacon cloud login --api-key usr_...` / `BEACON_API_KEY` for non-interactive use — saves the key and enables reporting. The default API base URL is compiled into the binary; self-hosted backends: `beacon cloud login --cloud-url https://your-host.example.com/api`.
3. Restart `beacon master` (or `beacon-master.service`). First successful heartbeat registers the device.

---

## Where files live (single Beacon home)

All agent state lives under **`~/.beacon`** unless **`BEACON_HOME`** is set (absolute path recommended). Typical layout:

```
~/.beacon/
  config.yaml              # Master identity + projects list (v2)
  config/
    projects/<id>/env      # Per-project deploy env
    projects/<id>/monitor.yml
    global.yml, agent.yml  # Legacy/auxiliary
  state/                   # Check results, deploy status
  ipc/                     # Master ↔ child
  keys/                    # Encrypted store used by `beacon keys` (not the BeaconInfra API key)
  templates/, logs/
```

- **`~/beacon`** (no leading dot) — default **clone/deploy working tree**, not the agent config root.
- **`~/.config/systemd/user/`** — user systemd units (`beacon@…`, `beacon-master`), normal on Linux.

Inspect paths: **`beacon config show`**.

---

## Two different “keys”

| Mechanism | Purpose |
|-----------|---------|
| **`api_key` in `~/.beacon/config.yaml`** | BeaconInfra user API key for **master heartbeats** (`beacon cloud login`, env `BEACON_API_KEY` for automation). |
| **`beacon keys …`** | Encrypted local store for **monitoring/Git** tokens (e.g. legacy reporting integrations). **Not** the same as the cloud dashboard API key. |

---

## What you get

### Terminal — `beacon status`

Connects to the master HTTP API (`http://127.0.0.1:9100/api/status` by default). Example output shape:

```
⬡ beacon v0.3.1-beta  ● master running  pid 1847  uptime 14d 3h

DEVICE  pi-homelab  192.168.1.42  arm64  Debian 12

SYSTEM  cpu 12% …  mem 67% …  disk 41% …

CHILDREN  3 healthy  1 warning  0 down
  …
```

### Browser — `http://localhost:9100`

Self-contained dashboard, `/metrics`, `/api/status`.

### Architecture

```
┌──────────────────────────────────────────────┐
│          BeaconInfra Cloud (optional)         │
└───────────────────┬──────────────────────────┘
                    │ HTTPS
┌───────────────────┴──────────────────────────┐
│             beacon master                     │
│  Spawns child agents, local dashboard, IPC.   │
└──────┬───────────────────────┬───────────────┘
       │  IPC (file-based)     │
┌──────┴──────────┐   ┌────────┴──────────┐
│  child agent    │   │  child agent      │  ...
└─────────────────┘   └───────────────────┘
```

---

## Set up a project (summary)

```bash
beacon bootstrap myapp
# or: beacon bootstrap myapp -f bootstrap.yml
```

Bootstrap creates project dirs, optional `beacon@myapp` systemd service, and registers **`projects:`** in `~/.beacon/config.yaml`. Add checks under `~/.beacon/config/projects/myapp/monitor.yml` (copy from [beacon.monitor.example.yml](./examples/) or use **`beacon setup-wizard`** as an optional helper to generate monitor YAML elsewhere).

### Bootstrap YAML (non-interactive)

See README examples in-repo; fields include `deployment_type`, `repo_url`, `local_path`, `deploy_command`, `poll_interval`, `secure_env_path`, etc.

### Cloud snippet (`~/.beacon/config.yaml`)

```yaml
api_key: "usr_…"                       # after you choose to use the cloud
device_name: "my-pi"                   # default: hostname
cloud_url: "https://beaconinfra.dev/api"
heartbeat_interval: 30
cloud_reporting_enabled: true
metrics_port: 9100

projects:
  - id: "myapp"
    config_path: "/home/user/.beacon/config/projects/myapp/monitor.yml"
```

Run **`beacon init`** for a local-only file (no API key), then **`beacon cloud login`** when you want the cloud.

---

## Command reference

| Command | Purpose |
|---------|---------|
| `beacon master` | Master agent: dashboard, children, optional heartbeats |
| `beacon status` | Terminal status from running master (`--json`, `--watch`, `--port`) |
| `beacon init` | Write local `config.yaml` (optional `--name`, `--metrics-port`; no network) |
| `beacon cloud login` / `beacon cloud logout` | Save API key and enable reporting / clear key and set `cloud_reporting_enabled: false` |
| `beacon config show` | Print `BEACON_HOME`, config path, device name, project count, API base URLs |
| `beacon bootstrap <name>` | New project layout + inventory + global `projects:` entry |
| `beacon deploy` | Deploy/poll loop (default when no subcommand) |
| `beacon monitor` | Single-project monitor (debug) |
| `beacon projects …` | List/add/status/remove projects (inventory) |
| `beacon keys …` | Local encrypted key store (not BeaconInfra `api_key`) |
| `beacon setup-wizard` | Interactive monitor YAML + env helper |
| `beacon alerts …` | Simple alerting CLI |
| `beacon template …` | Alert templates |
| `beacon source …` | Observation sources (e.g. Kubernetes) |
| `beacon mcp …` | MCP server for editors |
| `beacon version` | Version info |
| `beacon restart …` | Placeholder restart helper |

Hidden: `beacon agent` (spawned by master only).

---

## Environment variables

| Variable | Description |
|----------|-------------|
| `BEACON_HOME` | Override Beacon data directory (default: `~/.beacon`) |
| `BEACON_API_KEY` | User API key for `beacon cloud login` when not passed as a flag |
| `BEACON_DEVICE_NAME` | Default device name for `beacon init` / `beacon cloud login` when `--name` is omitted |
| `NO_COLOR` | Disable ANSI colors in `beacon status` |

---

## Run as a service

`beacon bootstrap` can install user systemd units. Manual master unit example:

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

## CI and quality

| Check | Description |
|-------|-------------|
| Tests | Go 1.24, all platforms |
| Platforms | Linux ARM / ARM64 / AMD64, macOS ARM64 / AMD64 |
| Security | gosec + govulncheck |
| Lint | golangci-lint |
| Coverage | [codecov](https://codecov.io/gh/Bajusz15/beacon) |

---

## Documentation

- [docs/MASTER_AGENT.md](./docs/MASTER_AGENT.md) — master agent and heartbeats
- [docs/LOG_FORWARDING.md](./docs/LOG_FORWARDING.md) — log forwarding
- [docs/E2E_TESTING.md](./docs/E2E_TESTING.md) — E2E tests
- [beacon.monitor.yml](./beacon.monitor.yml) — monitor schema reference
- [examples/](./examples/) — bootstrap and wizard examples

---

## License

Apache 2.0 — see [LICENSE](./LICENSE)
