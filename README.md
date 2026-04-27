# Beacon

<img src="./docs/logo.png" alt="Beacon Logo" width="120" height="120">

**🔧 One static binary on *your* box — ship from Git or Docker, watch health in the terminal *and* a real browser UI, stay 🔐 privacy-first. ☁️ Cloud is optional.**

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-lightgrey)](https://github.com/Bajusz15/beacon/releases)
[![CI](https://github.com/Bajusz15/beacon/workflows/CI/badge.svg)](https://github.com/Bajusz15/beacon/actions)
[![codecov](https://codecov.io/gh/Bajusz15/beacon/branch/main/graph/badge.svg)](https://codecov.io/gh/Bajusz15/beacon)
[![Go Report Card](https://goreportcard.com/badge/github.com/Bajusz15/beacon)](https://goreportcard.com/report/github.com/Bajusz15/beacon)

---

## 👋 Start here — what is Beacon, and why should I care?

**Beacon is for people who run their own apps** (homelab, Pi, N100, VPS): one agent that helps you **deploy** and **monitor** without giving up ownership of your stack.

Most tools assume you either want *only* CI/CD or *only* uptime pings. Beacon ties it together on the machine:

| Step | What you get |
|:---:|:---|
| 🚀 | **Automate releases** — point at **GitHub/Git** or **Docker/OCI** registries; Beacon **polls for new tags**, pulls, and runs **your** deploy script or Compose. Less SSH babysitting. |
| 📟 + 🖥️ | **Monitor local-first** — rich **`beacon status`** in the terminal *and* a **built-in dashboard** at **`http://localhost:9100`** (metrics, per-project health, Prometheus). No SaaS account required; works **offline**. |
| 🔐 | **Privacy by default** — **no account, no phone-home** unless *you* turn on cloud reporting. Tokens and app secrets stay **on disk you control**. |
| ☁️ | **Optional [BeaconInfra](https://beaconinfra.dev)** — free account → API key → `beacon cloud login`. Then you can get a **multi-device view**, **log forwarding**, and **browser-side deploy coordination** on top of the same agent — still your hardware underneath. |

That last step is **opt-in**. Skip it and Beacon never sends heartbeats or telemetry.

---

## 🤔 Still unsure what it does?

1. **“I want GitHub / Docker to deploy my app without me logging in every time.”**  
   → `beacon bootstrap` + `beacon deploy` (poll loop; often systemd). Git tags or new image tags trigger your commands.

2. **“I want to *see* that it’s healthy — terminal *and* a proper UI.”**  
   → `beacon start` serves a **clean local dashboard** (auto-refresh, no CDN). **`beacon status`** is the CLI view (`--watch` is great).

3. **“Okay, what if I want remote visibility / logs / coordination from a browser.”**  
   → Sign up at **BeaconInfra**, create an **API key**, run **`beacon cloud login`**, restart **`beacon start`**. First heartbeat registers the device — no separate “register this box” wizard. **`beacon cloud logout`** turns it all off again.

---

## ☁️ BeaconInfra in one minute

[BeaconInfra](https://beaconinfra.dev) is the **optional** hosted control plane: dashboard, API keys, heartbeats, and workflows that need a central place. It **adds** to Beacon; it doesn’t replace local monitoring.

```bash
beacon cloud login --api-key usr_xxxxxxxx   # or interactive: beacon cloud login
beacon start
```

The **first successful heartbeat** registers the device. To go fully local again: **`beacon cloud logout`** (clears the key and disables reporting).

---

## ⚡ Quick Start

### 1. Install

```bash
curl -fsSL https://raw.githubusercontent.com/Bajusz15/beacon/main/scripts/install.sh | bash
```

### 2. Initialize your device

```bash
beacon init --name my-pi
```

This creates `~/.beacon/config.yaml` with your device name, metrics port, and an empty project list. No network calls — everything stays local. If you skip `--name`, Beacon auto-detects your hostname.

### 3. Start Beacon

```bash
beacon start
```

This launches the Beacon agent: local dashboard at **http://localhost:9100**, system metrics, project management, and (if logged in) cloud heartbeats. Auto-refreshes, no external dependencies, works fully offline.

### 4. (Optional) Connect to BeaconInfra cloud

```bash
beacon cloud login --api-key usr_xxxxxxxx
beacon start   # restart to enable heartbeats
```

The first heartbeat registers your device automatically — no separate signup wizard. To disconnect: `beacon cloud logout`.

> **Note:** running plain **`beacon`** (no subcommand) prints **help**. For the full agent (dashboard, tunnels, heartbeats), use **`beacon start`**. For Git/Docker tag polling, use **`beacon deploy`**.

---

## ✨ What you get

Once **`beacon start`** is running (and you’ve added projects), you get:

### 🖥️ Terminal — `beacon status`

Connects to the running agent and shows a colored summary:

```
⬡ beacon v0.3.1-beta  ● running  pid 1847  uptime 14d 3h

DEVICE  pi-homelab  192.168.1.42  arm64  Debian 12

SYSTEM  cpu 12% ████░░░░░░░░░░░░  mem 67% ██████████░░░░░░  disk 41% ██████░░░░░░░░░░
        load 0.42 0.38 0.35  temp 48°C

PROJECTS  3 healthy  1 warning  0 down

  ● portfolio-site      v2.1.0   deployed 2h ago    3/3 checks passing
  ● home-assistant      v2024.3  deployed 5d ago    2/2 checks passing
  ◐ nextcloud           v28.0.1  deployed 3d ago    2/3 checks passing
    └─ ⚠ HTTP https://cloud.local/status  timeout 5.2s > 3s threshold
```

Flags: `--json`, `--watch` (refresh every 5s), `--no-color`, `--port <N>`.

### 🌐 Browser — `http://localhost:9100`

Self-contained HTML dashboard served by `beacon start`. No CDN, no external dependencies. Auto-refreshes every 10s.

- `/api/status` — JSON API
- `/metrics` — Prometheus format
- `/health` — simple health check

---

## 🧱 Set up a project

Beacon manages your apps end-to-end: clone from Git or pull from Docker registries, run your deploy command, poll for updates, health check, and tail logs. Each project runs as its own isolated agent process.

### 🧙 Interactive setup

```bash
beacon bootstrap myapp
```

The wizard asks for your deployment type (Git or Docker), repo URL, tokens, and deploy command. It creates a systemd service and **kicks off the first deployment** — then returns you to the terminal.

If systemd isn't available (containers, macOS, minimal installs), bootstrap still creates all the config files — just run `beacon deploy` yourself.

### 📄 From a config file

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

### 📦 What bootstrap creates

- `~/.beacon/config/projects/myapp/env` — deploy environment (tokens, paths, commands)
- `~/.beacon/config/projects/myapp/monitor.yml` — health check config
- `beacon@myapp.service` — systemd service that runs `beacon deploy` (skipped if systemd unavailable)
- Appends the project to `~/.beacon/config.yaml` so `beacon start` manages it

### 🔄 How deploy works

`beacon deploy` is a long-running process that polls for new releases and deploys automatically. Bootstrap sets it up as a systemd service, but you can also run it directly.

**Git mode:** every `poll_interval`, runs `git fetch --tags`, detects the newest tag, clones it, and runs your `deploy_command`. Auth supports HTTPS tokens (GitHub, GitLab) and SSH keys.

**Docker mode — how it actually works**

1. **You configure an image repository** (e.g. `ghcr.io/you/web-app` or `username/app` on Docker Hub) in `docker_images` — **not** a manual “watch this compose file’s `latest` tag” toggle. Beacon talks to the **registry HTTP API**, lists tags for that repository, and picks the **newest tag** (semantic-version style when possible, otherwise a stable sort).
2. On each **`poll_interval`**, it compares that tag to the **last deployed tag** stored under `~/.beacon/state/...`. If the tag **changed** (or it’s the first run), Beacon:
   - runs **`docker pull`** for `image:tag`, then  
   - runs **your** `deploy_command` (e.g. `docker compose up -d`) from the right working directory.
3. **Docker Compose** is optional but common: put your `docker-compose.yml` (and overrides) under **`local_path`**, list them in `docker_compose_files`. The deploy command is usually “bring stack up after pull” — Compose still references **your** service image names; Beacon has already pulled the tag it detected. You can use **`${BEACON_DOCKER_IMAGE}`** in custom commands when you need the exact ref the poller chose.

Environment variables passed into `deploy_command`:

| Variable | Example |
|----------|---------|
| `BEACON_DOCKER_IMAGE` | `ghcr.io/you/app:v1.2.0` |
| `BEACON_DOCKER_TAG` | `v1.2.0` |
| `BEACON_DOCKER_COMPOSE_FILES` | `docker-compose.yml docker-compose.prod.yml` |

### 🩺 Health checks

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

Wire the project into `~/.beacon/config.yaml`:

```yaml
projects:
  - id: "myapp"
    config_path: "/home/user/.beacon/config/projects/myapp/monitor.yml"
```

Restart Beacon and the project appears in `beacon status` and the dashboard.

### 🔑 Application secrets

`secure_env_path` in your bootstrap config points to **your application's** environment file — where you keep `DATABASE_URL`, `API_SECRET`, etc. Beacon loads it at deploy time so your app has its secrets. This file is separate from Beacon's own config.

---

## ☁️ Cloud dashboard (optional)

Same story as **☁️ BeaconInfra in one minute** (section above): **additive** multi-device / logs / coordination — only after **`beacon cloud login`**. **🔐 With no API key, Beacon makes zero outbound reporting calls.**

```bash
beacon cloud login --api-key usr_abc123def456
beacon start
```

Self-hosted backend: build from source with `go build -ldflags "-X beacon/internal/cloud.DefaultBeaconInfraAPIURL=https://your-host.example.com/api"`. Disable cloud: **`beacon cloud logout`**.

### 📝 `~/.beacon/config.yaml`

Created by `beacon init`. You can also edit it directly.

```yaml
api_key: "usr_abc123def456"       # set by beacon cloud login (omit for offline)
device_name: "my-pi"              # defaults to hostname
heartbeat_interval: 30            # seconds (cloud URL is compile-time only)
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

## 📁 Where files live

All state lives under **`~/.beacon`** (override with `BEACON_HOME`):

```
~/.beacon/
  config.yaml                    # Agent config + project list
  config/projects/<id>/env       # Per-project deploy environment
  config/projects/<id>/monitor.yml
  state/                         # Check results, deploy status
  ipc/                           # Agent <-> project agent communication
  keys/                          # Encrypted token store (beacon keys)
  logs/
```

Note: `~/beacon` (no dot) is the default **deploy working tree** for cloned repos, not the agent config root.

Inspect paths: `beacon config show`

---

## ⌨️ Commands

| Command | Purpose |
|---------|---------|
| `beacon start` | Start Beacon (dashboard at :9100, manages projects + tunnels, optional heartbeats) |
| `beacon status` | Terminal health view from running agent (`--json`, `--watch`, `--no-color`) |
| `beacon init` | Write local `config.yaml` (`--name`, `--metrics-port`; no network) |
| `beacon cloud login` / `logout` | Enable/disable cloud reporting |
| `beacon config show` | Print resolved paths, identity, and project count |
| `beacon bootstrap <name>` | Set up a new project (interactive or `-f config.yml`) |
| `beacon deploy` | Git/Docker tag polling loop (must be run explicitly) |
| `beacon monitor [-f config.yml]` | Run one project's health checks (debug) |
| `beacon projects list\|add\|remove\|status\|info` | Project inventory management |
| `beacon tunnel add\|list\|enable\|disable` | Manage reverse tunnels for remote access to local services |
| `beacon vpn enable` | Turn this device into a WireGuard exit node (requires port-forward for 51820/UDP) |
| `beacon vpn use <device>` | Connect to another Beacon device's VPN exit node |
| `beacon vpn disable` | Tear down VPN interface and deregister |
| `beacon keys list\|add\|rotate\|delete\|validate` | Encrypted local token store |
| `beacon alerts init\|status\|test\|acknowledge\|resolve` | Alert routing (webhook JSON + optional SMTP; `--project` required) |
| `beacon setup-wizard` | Interactive monitor YAML + env helper |
| `beacon source add\|list\|remove\|status` | Observation sources (e.g. Kubernetes) |
| `beacon mcp serve` | MCP server for Cursor / Claude Desktop |
| `beacon version` | Version info |

Hidden: `beacon agent` (project agent process, spawned internally by `beacon start`).

---

## 🧪 Environment variables

| Variable | Description |
|----------|-------------|
| `BEACON_HOME` | Override data directory (default: `~/.beacon`) |
| `BEACON_API_KEY` | API key for `beacon cloud login` when not passed as a flag |
| `BEACON_DEVICE_NAME` | Device name when `--name` is omitted |
| `NO_COLOR` | Disable ANSI colors in `beacon status` |
| `WEBHOOK_URL` | Expanded in `~/.beacon/config/projects/<id>/alerts.yml` for outbound webhook alerts |
| `SMTP_USER` / `SMTP_PASSWORD` | SMTP credentials when using email in `alerts.yml` |

---

## ⚙️ Run as a service

`beacon bootstrap` installs systemd services automatically. For manual setup:

```bash
cat > ~/.config/systemd/user/beacon-master.service << 'EOF'
[Unit]
Description=Beacon Master Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/beacon start
Restart=on-failure
RestartSec=30

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now beacon-master.service
```

---

---

## 📚 Documentation

- [docs/MASTER_AGENT.md](./docs/MASTER_AGENT.md) — agent architecture and heartbeats
- [docs/LOG_FORWARDING.md](./docs/LOG_FORWARDING.md) — log forwarding configuration
- [docs/KEY_MANAGEMENT.md](./docs/KEY_MANAGEMENT.md) — encrypted key store
- [docs/MCP.md](./docs/MCP.md) — MCP server for editors
- [docs/E2E_TESTING.md](./docs/E2E_TESTING.md) — E2E test setup
- [beacon.monitor.yml](./beacon.monitor.yml) — monitoring config reference
- [examples/](./examples/) — bootstrap, monitor, alert, and wizard examples

---

## 🏗️ Architecture

`beacon start` runs a single orchestrator process per device. Internally we call this the **master** — it manages everything else but stays deliberately simple: collect system metrics, serve the local dashboard, send heartbeats, and supervise project agents.

The master is **stateless per project** — it doesn't know about Docker or systemd. Each project runs as its own isolated agent process; one crash doesn't affect others. The master auto-restarts failed projects with exponential backoff. Tunnels run as lightweight goroutines inside the master, connecting outbound to the cloud via WebSocket so local services are accessible without opening ports.

```
┌──────────────────────────────────────────────────────┐
│             BeaconInfra Cloud (optional)              │
│  heartbeats, commands, tunnel proxy, terminal relay   │
└──────────┬───────────────────────────┬───────────────┘
           │ HTTPS                     │ WebSocket
┌──────────┴───────────────────────────┴───────────────┐
│                  beacon start                        │
│                  (the "master")                       │
│                                                       │
│  One per device. Collects system metrics, serves      │
│  local dashboard (:9100), sends heartbeats.           │
│  Manages project agents (processes), tunnels          │
│  (goroutines), VPN (WireGuard), and remote terminal.  │
└──┬──────────────┬──────────────┬──────────┬──────────┘
   │ IPC          │ IPC          │ goroutine │ WireGuard
┌──┴───────────┐ ┌┴────────────┐ ┌┴─────────┐ ┌┴─────────┐
│ project agent│ │project agent│ │ tunnels   │ │ VPN      │
│ myapp        │ │ blog        │ │ HA  :8123 │ │ beacon0  │
│ health checks│ │ health check│ │ NC  :8080 │ │ 51820/UDP│
│ log tailing  │ │ log tailing │ │ (WS proxy)│ │          │
└──────────────┘ └─────────────┘ └───────────┘ └──────────┘
```

### BeaconInfra “home away” tunnel (end user)

1. `beacon cloud login` with your **usr_** API key (same account as the dashboard).
2. Add a tunnel: `beacon tunnel add homeassistant --port 8123` (id and port as needed).
3. Run `beacon start` (or restart your systemd unit). Tunnels connect automatically at startup and stay connected with exponential backoff reconnect.
4. For **Home Assistant**, add `127.0.0.1` to `http` → `trusted_proxies` and use `use_x_forwarded_for: true` so URLs and sessions work behind the tunnel.

### WireGuard VPN

Beacon can turn any device into a WireGuard exit node so other devices route traffic through it — useful for accessing your home network from a laptop or phone.

```bash
# On your home device (needs a port-forwarded UDP port)
beacon vpn enable --listen-port 51820

# On your laptop or another Beacon device
beacon vpn use my-home-pi
```

BeaconInfra coordinates the key exchange (endpoint, public keys) — the actual tunnel is peer-to-peer WireGuard. For laptops/desktops that only need client mode, install the standalone `beacon-vpn` binary instead of the full agent: `beacon-vpn connect my-home-pi`.

Tear down: `beacon vpn disable`.

---

## 📖 What you can do with Beacon

Beacon does a lot in one binary. The tables below are a quick tour — if something looks useful, the sections above cover the full setup.

### On your own machine (no account needed)

Everything here works without an internet connection and without signing up for anything.

| You want to… | How |
|---|---|
| **Deploy a Git repo automatically** when you push a new tag | `beacon bootstrap myapp` — point it at your repo, give it your deploy script. Beacon polls for new tags and runs the script. |
| **Deploy Docker images automatically** from Docker Hub, GHCR, or any private registry | Use `deployment_type: docker` in your bootstrap config. Beacon watches for new tags and runs your `docker compose up -d` (or anything else). |
| **Deploy a whole stack** where each image moves independently | List multiple images under `docker_images:` in your bootstrap. Only the image that changed redeploys. |
| **Check that an HTTP endpoint is up** | Add an HTTP check to your project's `monitor.yml` — set a URL, an interval, and a timeout. |
| **Check that a port is open** (databases, SSH, custom services) | Add a `type: port` check with a host and port. |
| **Check anything a shell command can check** | Add a `type: command` check. The exit code tells Beacon if it's up. |
| **See everything at a glance from the terminal** | `beacon status` — colored summary of every project. Add `--watch` for a live view. |
| **See everything in a browser** | Open `http://<your-device>:9100`. Self-contained dashboard that auto-refreshes. |
| **Pull metrics into Grafana / Prometheus** | Scrape `http://<your-device>:9100/metrics`. |
| **See CPU, memory, disk, load, temperature** | Enabled by default. Shows up in `beacon status` and the dashboard. |
| **Get a Slack / Discord / webhook message when something goes down** | Create `alerts.yml` next to your `monitor.yml`. Route by severity to any webhook. |
| **Get an email when something goes down** | Same `alerts.yml`, add an `email` channel with your SMTP details. |
| **Silence alerts at night** | Add `quiet_hours:` to your alert routing with a start/end time and timezone. |
| **Test your alert setup without waiting for an outage** | `beacon alerts test --project myapp --severity critical` |
| **Forward logs** from a file, a Docker container, or `journalctl` | Add a `log_sources:` block to your `monitor.yml`. Filter with include/exclude patterns so you only ship what you care about. |
| **Keep your tokens out of config files** | `beacon keys add` — encrypted local token store for Git, Docker, webhooks. |
| **Expose Home Assistant, Grafana, or any local service to the outside world** (with a BeaconInfra account — no port-forwarding needed) | `beacon tunnel add homeassistant --port 8123` |
| **Run several tunnels at once** | `beacon tunnel list` / `beacon tunnel enable` / `beacon tunnel disable` |
| **Route traffic through your home network from your laptop** | `beacon vpn enable` on the exit node, `beacon vpn use <device>` on the client. Peer-to-peer WireGuard. |
| **Query Beacon from Cursor or Claude Desktop** | `beacon mcp serve` — see [docs/MCP.md](./docs/MCP.md) |
| **Monitor a Kubernetes cluster** | `beacon source add` with your kubeconfig. |
| **Manage your project list** | `beacon projects list`, `beacon projects add`, `beacon projects status myapp` |

### With a BeaconInfra account (optional)

A free BeaconInfra account adds a hosted dashboard and remote access on top of everything above. Your device keeps running locally — the cloud just gives you somewhere to see it all from a browser, including from your phone.

Turn it on with `beacon cloud login --api-key usr_…`. Turn it off any time with `beacon cloud logout`.

| You want to… | What you get |
|---|---|
| **See all your devices in one place** | One dashboard showing every machine running Beacon — your Pi, your NAS, your VPS, your homelab server — with current health, uptime, and system metrics. |
| **Open Home Assistant from your phone, anywhere** | Set up the `homeassistant` tunnel once. From then on, open the BeaconInfra dashboard on your phone and click through to your HA UI. No VPN, no port-forwarding, no dynamic DNS. |
| **Reach any other local service remotely** | The same tunnel mechanism works for Grafana, Jellyfin, Pi-hole, your router's admin page, a staging VM — anything that speaks HTTP on your LAN. |
| **View logs from anywhere** | The log lines you configured to forward show up in the dashboard, filterable by device and project. Useful when something breaks and you don't want to SSH in. |
| **Watch your metrics remotely** | CPU, memory, disk, load, and temperature for every device — without being on the LAN. |
| **See all your project health in one list** | Every project, every check, across every device. Sorted so the problems come first. |
| **Trigger a deploy from the browser** | Click "deploy" in the dashboard and Beacon runs your existing deploy script on the device. Your secrets never leave home — the cloud just sends the signal. |
| **Know when a device goes offline** | If a device stops sending heartbeats, you get notified — even if its last check said everything was fine. |
| **Open a remote terminal session** | Click "Open terminal" on a device in the dashboard. The cloud relays a shell session (PTY) between your browser and the Beacon agent — no SSH port, no VPN needed. Sessions auto-expire after 15 minutes. |
| **Route traffic through your home network** | `beacon vpn enable` on your home device + `beacon vpn use my-pi` on your laptop. WireGuard peer exchange happens via BeaconInfra; the actual traffic is peer-to-peer. For client-only machines, use the lightweight `beacon-vpn` binary. |

### What we don't see

Even with BeaconInfra enabled, some things stay on your device and never touch the cloud:

- Your **source code** and **deploy scripts** — the cloud only sends a "deploy now" signal; your device runs the script.
- Your **tokens** (Git, Docker, webhooks) — encrypted locally by `beacon keys`.
- Your **application secrets** (database passwords, API keys loaded via `secure_env_path`) — Beacon hands them to your app at deploy time and nothing else.
- **Raw log files** — only the lines you explicitly configured as `log_sources` are forwarded. Everything else stays on disk.
- The **local dashboard** at port 9100 — it keeps working offline, BeaconInfra account or not.

If you change your mind, `beacon cloud logout` stops all outbound reporting on the next heartbeat. There's nothing to delete from a control panel because there's no persistent account state beyond what you chose to send.

---

☕ **[Buy me a coffee](https://buymeacoffee.com/matebajusz)** — if Beacon saves you time.

---

## 📄 License

Apache 2.0 — see [LICENSE](./LICENSE)
