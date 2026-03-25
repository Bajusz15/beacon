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

## Local Dashboard

When `beacon master` is running, a local metrics dashboard is available at **http://localhost:9100**.

No cloud account required — fully offline, localhost-only.

### `beacon status` — terminal view

<div style="background: #0A1628; border-radius: 12px; padding: 20px 24px; font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace; font-size: 13px; line-height: 1.6; color: #CBD5E1; overflow-x: auto;">
<div style="display:flex;align-items:center;gap:8px;margin-bottom:16px;padding-bottom:12px;border-bottom:1px solid rgba(255,255,255,0.06)">
<div style="width:10px;height:10px;border-radius:50%;background:#ef4444"></div>
<div style="width:10px;height:10px;border-radius:50%;background:#F59E0B"></div>
<div style="width:10px;height:10px;border-radius:50%;background:#06B6D4"></div>
<span style="color:#64748B;font-size:12px;margin-left:8px">~ beacon status</span>
</div>
<pre style="margin:0;white-space:pre;overflow-x:auto;color:#CBD5E1"><span style="color:#F59E0B;font-weight:600">⬡ beacon</span> <span style="color:#64748B">v0.3.1-beta</span>  <span style="color:#06B6D4">●</span> master running  <span style="color:#64748B">pid 1847  uptime 14d 3h</span>

<span style="color:#94A3B8">DEVICE</span>  <span style="color:#fff">pi-homelab</span>  <span style="color:#64748B">192.168.1.42  arm64  Debian 12</span>

<span style="color:#94A3B8">SYSTEM</span>  <span style="color:#64748B">cpu</span> <span style="color:#06B6D4">12%</span> <span style="color:#162D50">████</span><span style="color:#0F1F3A">████████████</span>  <span style="color:#64748B">mem</span> <span style="color:#F59E0B">67%</span> <span style="color:#162D50">██████████</span><span style="color:#0F1F3A">█████</span>  <span style="color:#64748B">disk</span> <span style="color:#06B6D4">41%</span> <span style="color:#162D50">██████</span><span style="color:#0F1F3A">█████████</span>
        <span style="color:#64748B">load</span> <span style="color:#CBD5E1">0.42 0.38 0.35</span>  <span style="color:#64748B">temp</span> <span style="color:#CBD5E1">48°C</span>

<span style="color:#94A3B8">CHILDREN</span>  <span style="color:#06B6D4">3 healthy</span>  <span style="color:#F59E0B">1 warning</span>  <span style="color:#ef4444">0 down</span>

  <span style="color:#06B6D4">●</span> <span style="color:#fff">portfolio-site</span>      <span style="color:#64748B">v2.1.0   deployed 2h ago    3/3 checks passing</span>
  <span style="color:#06B6D4">●</span> <span style="color:#fff">home-assistant</span>       <span style="color:#64748B">v2024.3  deployed 5d ago    2/2 checks passing</span>
  <span style="color:#06B6D4">●</span> <span style="color:#fff">pi-hole</span>              <span style="color:#64748B">v5.18    deployed 12d ago   4/4 checks passing</span>
  <span style="color:#F59E0B">◐</span> <span style="color:#fff">nextcloud</span>            <span style="color:#F59E0B">v28.0.1  deployed 3d ago    2/3 checks passing</span>
    <span style="color:#F59E0B">└─ ⚠ HTTP https://cloud.local/status  timeout 5.2s > 3s threshold</span>

<span style="color:#94A3B8">RECENT</span>  <span style="color:#64748B">last 24h</span>
  <span style="color:#64748B">14:32</span>  <span style="color:#06B6D4">deploy</span>   portfolio-site v2.1.0 → success <span style="color:#64748B">(12s)</span>
  <span style="color:#64748B">11:07</span>  <span style="color:#F59E0B">alert</span>    nextcloud HTTP check timeout <span style="color:#64748B">(3 consecutive)</span>
  <span style="color:#64748B">09:00</span>  <span style="color:#06B6D4">heartbeat</span> cloud sync OK <span style="color:#64748B">(metrics + logs)</span>

<span style="color:#64748B">metrics</span> <span style="color:#0D9488">http://localhost:9100</span>  <span style="color:#64748B">prometheus</span> <span style="color:#0D9488">http://localhost:9100/metrics</span></pre>
</div>

### Browser dashboard — `http://localhost:9100`

<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  .dash { background: #0A1628; border-radius: 12px; padding: 20px; font-family: 'Inter', system-ui, sans-serif; color: #CBD5E1; min-height: 500px; }
  .dash-hdr { display: flex; align-items: center; justify-content: space-between; margin-bottom: 20px; padding-bottom: 14px; border-bottom: 1px solid rgba(255,255,255,0.06); }
  .dash-brand { display: flex; align-items: center; gap: 10px; }
  .dash-brand-icon { width: 28px; height: 28px; background: rgba(245,158,11,0.15); border-radius: 6px; display: flex; align-items: center; justify-content: center; }
  .dash-brand-icon svg { width: 16px; height: 16px; }
  .dash-brand-name { font-size: 15px; font-weight: 600; color: #F59E0B; }
  .dash-brand-ver { font-size: 11px; color: #64748B; }
  .dash-brand-host { font-size: 12px; color: #94A3B8; }
  .dash-uptime { font-size: 11px; color: #64748B; text-align: right; }
  .dash-uptime span { color: #06B6D4; font-weight: 500; }
  .metric-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; margin-bottom: 16px; }
  .metric-card { background: #0F1F3A; border-radius: 8px; padding: 12px 14px; }
  .metric-label { font-size: 11px; color: #64748B; margin-bottom: 4px; text-transform: uppercase; letter-spacing: 0.5px; }
  .metric-val { font-size: 22px; font-weight: 600; color: #fff; }
  .metric-bar { height: 4px; border-radius: 2px; background: #162D50; margin-top: 8px; overflow: hidden; }
  .metric-fill { height: 100%; border-radius: 2px; }
  .section-title { font-size: 12px; color: #64748B; text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 10px; margin-top: 4px; }
  .child-row { display: flex; align-items: center; gap: 10px; padding: 10px 14px; background: #0F1F3A; border-radius: 8px; margin-bottom: 6px; }
  .child-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
  .child-name { font-size: 13px; font-weight: 500; color: #fff; min-width: 120px; }
  .child-ver { font-size: 11px; color: #64748B; min-width: 60px; }
  .child-checks { font-size: 11px; color: #94A3B8; margin-left: auto; }
  .child-checks-ok { color: #06B6D4; }
  .child-checks-warn { color: #F59E0B; }
  .child-deployed { font-size: 11px; color: #64748B; min-width: 90px; }
  .child-warn-detail { font-size: 11px; color: #F59E0B; padding: 6px 14px 6px 32px; margin-top: -2px; margin-bottom: 6px; }
  .event-row { display: flex; align-items: baseline; gap: 10px; padding: 6px 0; font-size: 12px; }
  .event-time { color: #64748B; min-width: 40px; font-family: 'SF Mono', monospace; font-size: 11px; }
  .event-type { font-size: 11px; padding: 2px 8px; border-radius: 4px; font-weight: 500; }
  .event-msg { color: #94A3B8; }
  .events-section { background: #0F1F3A; border-radius: 8px; padding: 12px 14px; margin-top: 16px; }
  .footer-links { display: flex; gap: 16px; margin-top: 16px; padding-top: 12px; border-top: 1px solid rgba(255,255,255,0.06); font-size: 11px; }
  .footer-links a { color: #0D9488; text-decoration: none; }
</style>
<div class="dash">
  <div class="dash-hdr">
    <div class="dash-brand">
      <div class="dash-brand-icon"><svg viewBox="0 0 16 16" fill="none"><circle cx="8" cy="8" r="3" fill="#F59E0B"/><circle cx="8" cy="8" r="6" stroke="#F59E0B" stroke-width="1" stroke-opacity="0.3"/></svg></div>
      <div>
        <div style="display:flex;align-items:baseline;gap:8px"><span class="dash-brand-name">beacon</span><span class="dash-brand-ver">v0.3.1-beta</span></div>
        <div class="dash-brand-host">pi-homelab &middot; 192.168.1.42 &middot; arm64</div>
      </div>
    </div>
    <div class="dash-uptime">master uptime<br><span>14d 3h 22m</span></div>
  </div>
  <div class="metric-grid">
    <div class="metric-card">
      <div class="metric-label">CPU</div>
      <div class="metric-val" style="color:#06B6D4">12%</div>
      <div class="metric-bar"><div class="metric-fill" style="width:12%;background:#06B6D4"></div></div>
    </div>
    <div class="metric-card">
      <div class="metric-label">Memory</div>
      <div class="metric-val" style="color:#F59E0B">67%</div>
      <div class="metric-bar"><div class="metric-fill" style="width:67%;background:#F59E0B"></div></div>
    </div>
    <div class="metric-card">
      <div class="metric-label">Disk</div>
      <div class="metric-val" style="color:#06B6D4">41%</div>
      <div class="metric-bar"><div class="metric-fill" style="width:41%;background:#06B6D4"></div></div>
    </div>
    <div class="metric-card">
      <div class="metric-label">Load avg</div>
      <div class="metric-val">0.42</div>
      <div style="font-size:11px;color:#64748B;margin-top:8px">0.38 &middot; 0.35</div>
    </div>
  </div>
  <div class="section-title">Children &middot; <span style="color:#06B6D4">3 healthy</span> &middot; <span style="color:#F59E0B">1 warning</span></div>
  <div class="child-row">
    <div class="child-dot" style="background:#06B6D4"></div>
    <div class="child-name">portfolio-site</div>
    <div class="child-ver">v2.1.0</div>
    <div class="child-deployed">2h ago</div>
    <div class="child-checks child-checks-ok">3/3 passing</div>
  </div>
  <div class="child-row">
    <div class="child-dot" style="background:#06B6D4"></div>
    <div class="child-name">home-assistant</div>
    <div class="child-ver">v2024.3</div>
    <div class="child-deployed">5d ago</div>
    <div class="child-checks child-checks-ok">2/2 passing</div>
  </div>
  <div class="child-row">
    <div class="child-dot" style="background:#06B6D4"></div>
    <div class="child-name">pi-hole</div>
    <div class="child-ver">v5.18</div>
    <div class="child-deployed">12d ago</div>
    <div class="child-checks child-checks-ok">4/4 passing</div>
  </div>
  <div class="child-row" style="border-left:2px solid #F59E0B;border-radius:0 8px 8px 0">
    <div class="child-dot" style="background:#F59E0B"></div>
    <div class="child-name">nextcloud</div>
    <div class="child-ver">v28.0.1</div>
    <div class="child-deployed">3d ago</div>
    <div class="child-checks child-checks-warn">2/3 passing</div>
  </div>
  <div class="child-warn-detail">HTTP https://cloud.local/status &mdash; timeout 5.2s (threshold: 3s)</div>
  <div class="events-section">
    <div class="section-title" style="margin-top:0">Recent events</div>
    <div class="event-row">
      <span class="event-time">14:32</span>
      <span class="event-type" style="background:rgba(6,182,212,0.15);color:#06B6D4">deploy</span>
      <span class="event-msg">portfolio-site v2.1.0 deployed <span style="color:#64748B">(12s)</span></span>
    </div>
    <div class="event-row">
      <span class="event-time">11:07</span>
      <span class="event-type" style="background:rgba(245,158,11,0.15);color:#F59E0B">alert</span>
      <span class="event-msg">nextcloud HTTP check timeout <span style="color:#64748B">(3 consecutive)</span></span>
    </div>
    <div class="event-row">
      <span class="event-time">09:00</span>
      <span class="event-type" style="background:rgba(6,182,212,0.15);color:#06B6D4">sync</span>
      <span class="event-msg">cloud heartbeat OK <span style="color:#64748B">(metrics + logs)</span></span>
    </div>
  </div>
  <div class="footer-links">
    <a>/metrics</a>
    <a>/health</a>
    <span style="color:#64748B">prometheus :9100/metrics &middot; json :9100/api/status</span>
  </div>
</div>

### Flags

```bash
beacon status              # colored terminal output
beacon status --json       # raw JSON (for scripting / jq)
beacon status --no-color   # plain text (also respects NO_COLOR env var)
beacon status --watch      # auto-refresh every 5s
beacon status --port 9200  # custom port (if metrics_port set in config)
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
