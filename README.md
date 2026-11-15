# Beacon

<img src="./docs/logo.png" alt="Beacon Logo" width="120" height="120">

**Self-hosted infrastructure — secure remote access, monitoring, and tunneling platform.**

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-lightgrey)](https://github.com/Bajusz15/beacon/releases)
[![CI](https://github.com/Bajusz15/beacon/workflows/CI/badge.svg)](https://github.com/Bajusz15/beacon/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/Bajusz15/beacon)](https://goreportcard.com/report/github.com/Bajusz15/beacon)

---

You run things on machines you don't always sit in front of — Home Assistant on a Pi, a NAS, Pi-hole, Grafana, your own apps on an N100 or a VPS, a client's server. Beacon does two things first: it gives you **secure remote access** to those machines and the services on them — with no open ports — and it **monitors** their health and alerts you when something breaks. It's workload-agnostic: it reaches and watches whatever is on the box, not just things it deployed. (It can run your deploys too, but access and monitoring are the core.)

And it's built on a strict **local-first, zero-trust model** — your code, secrets, and data never leave the box, and no cloud ever holds root on your machines.

```
⬡ beacon v0.3.2  ● running  pid 1847  uptime 14d 3h

DEVICE  pi-homelab  192.168.1.42  arm64  Debian 12

SYSTEM  cpu 12% ████░░░░░░░░░░░░  mem 67% ██████████░░░░░░  disk 41% ██████░░░░░░░░░░
        load 0.42 0.38 0.35  temp 48°C

PROJECTS  3 healthy  1 warning  0 down

  ● portfolio-site      v2.1.0   deployed 2h ago    3/3 checks passing
  ● home-assistant      v2024.3  deployed 5d ago    2/2 checks passing
  ◐ nextcloud           v28.0.1  deployed 3d ago    2/3 checks passing
    └─ ⚠ HTTP https://cloud.local/status  timeout 5.2s > 3s threshold
```

That's `beacon status`. There's also a browser dashboard at `http://localhost:9100` — self-contained, no CDN, works offline.

---

## ✨ What Beacon does

Two things first — **secure remote access** and **monitoring** — on a zero-trust foundation. Everything else is a bonus.

### 🔐 Built zero-trust, local-first

This is the part that matters before any feature:

- **No open ports.** Tunnels connect *outbound* from your device — nothing is exposed to the internet, and it works behind CGNAT.
- **No control plane with root.** The optional cloud can never log in to your machine on its own; it only relays commands you authorized, and the agent runs them.
- **Nothing sensitive leaves the box.** Your source code, deploy scripts, tokens, and secrets stay local. The cloud only ever sees the metrics and log lines you explicitly choose to send.
- **Works offline.** The agent and its dashboard run fully local, with no account.

### 🌐 Remote access

- **Reach any service, no open ports** — securely reach Home Assistant, Grafana, Jellyfin, a NAS admin page, Pi-hole, or any local HTTP service from anywhere. No dynamic DNS, no Nabu Casa, no Cloudflare account or managed domain. Authenticated with short-lived tokens; only you can reach your services.
- **Remote terminal** — open a shell on your device from the browser. No SSH port, no VPN. The cloud relays a PTY session between your browser and the agent.
- **Device-verified gate (recommended)** — enroll a **passkey** (preferred) or set a passphrase (fallback) so a second factor is required before *any* remote terminal or tunnel session can open. It's verified **on the device** — BeaconInfra only relays. The passkey's private key never enters the page, and you can layer on an **authenticator-app (TOTP) code** so the cloud can't silently open a session. See [Securing remote access](#-securing-remote-access).

### 📊 Monitoring & alerts

- **Health & metrics** — health checks (HTTP, port, command), CPU/memory/disk/temperature, per-project status, Prometheus metrics.
- **Alerts** — via webhook (Slack, Discord) or SMTP, with quiet hours and offline-device detection.
- **Log forwarding** — tail log files, Docker container logs, or `journalctl` and forward them to the dashboard, filtered so you only ship what matters.

### 🧰 Also

- **Deploy automation (bring your own script)** — Beacon doesn't replace your stack or turn your box into a PaaS. Point it at a Git repo or Docker registry; it watches for new tags, injects your secrets, and runs *your* deploy script. Push a tag, walk away.
- **WireGuard VPN** — turn any Beacon device into a peer-to-peer WireGuard exit node and route traffic through your home network from a laptop.

[BeaconInfra](https://beaconinfra.dev) is the optional cloud that adds the multi-device dashboard, tunnels, and remote terminal. When an API key is configured, `beacon start` sends periodic heartbeats with device metrics and project health; the cloud uses these to power the dashboard, detect offline devices, and relay commands back to the agent. Everything except tunnels and remote terminal works without an account.

---

## 🧩 One layer across every machine

Beacon isn't a PaaS and doesn't replace how you deploy. It's the operations layer **across all your
machines** — the box running your apps *and* the Home Assistant box, the NAS, the Pis, the client
server, the VPS you never containerized — so you reach, watch, and fix every one of them from a
single place, without handing any cloud root over your hardware.

---

## 🏠 Access Home Assistant from anywhere

If you run Home Assistant OS, the primary path is the Beacon Home Assistant add-on:
install the add-on, paste your BeaconInfra API key, enable `tunnel_home_assistant`,
and start it. The add-on points the tunnel at Home Assistant Core on
`http://homeassistant:8123`.

If you run Beacon on a normal Linux host or in Docker next to Home Assistant, use
the CLI tunnel flow. Three commands, no port-forwarding, no Nabu Casa.

```bash
# 1. Log in to BeaconInfra (free account)
beacon cloud login --api-key bci_live_xxxxxxxx

# 2. Expose Home Assistant
beacon tunnel add homeassistant --port 8123
# Or, for Docker Compose / HA OS style service DNS:
# beacon tunnel add homeassistant --port 8123 --host homeassistant

# 3. Start Beacon
beacon start
```

Your Home Assistant is now accessible from the BeaconInfra dashboard — on your phone, from a hotel, wherever. The tunnel connects outbound from your device (no inbound ports needed), reconnects automatically, and works behind CGNAT.

The same tunnel works for Grafana, Jellyfin, Pi-hole, Nextcloud, your NAS admin page, a staging server — anything that speaks HTTP on your LAN.

**Home Assistant setup:** HA blocks proxied requests by default. Add this to your `configuration.yaml` (the file inside your HA config volume):

```yaml
http:
  use_x_forwarded_for: true
  trusted_proxies:
    - 172.30.0.0/16
    - 172.16.0.0/12
    - 127.0.0.1
```

Then restart Home Assistant Core. Without this, the tunnel connects but HA can return `400 Bad Request` or show "can't connect to Home Assistant" because it rejects the forwarded proxy headers.

---

## 🖥️ Proxmox — oversee the whole host

Run Beacon **on a Proxmox VE host** and it becomes an *overseer*: it monitors the host and
sees every VM and container the host runs — including the up/down state a crashed guest
can't report for itself — through the host's own `pvesh`. In the dashboard you get one card
per host with its guests nested underneath, live status, and **Start / Reboot / Shutdown**
controls. It's the one vantage point that survives a guest falling over.

**One-step install** — run on the Proxmox host, as root:

```bash
bash -c "$(curl -fsSL https://raw.githubusercontent.com/Bajusz15/beacon/main/scripts/proxmox-install.sh)"
```

The installer verifies it's a Proxmox host, downloads the latest Beacon binary, asks for your
BeaconInfra API key, registers the host, and starts the `beacon-master` systemd service. For an
unattended install, prefix the command with `BEACON_API_KEY=bci_...` (optionally
`BEACON_DEVICE_NAME="rack-pve-1"`).

Then, on the host:

```bash
beacon proxmox list      # every VM/CT: VMID, name, type, status
beacon proxmox status    # e.g. "3/4 guests running on this host."
```

The host now shows up in the BeaconInfra dashboard under **Devices** with its guests. Install
Beacon inside a guest too (e.g. the Home Assistant add-on) and the dashboard offers to link
that device under its host — so your whole box appears as one tree, with power controls on
each guest.

---

## 🔑 Securing remote access

BeaconInfra relays remote terminal and tunnel sessions to your device. Configure
a device-side remote-access factor to require your approval before a session can
open. Enrollment and unlocking are verified by the Beacon agent on that device.

| Factor | What you use | Setup |
| --- | --- | --- |
| **Passkey (recommended)** | Touch ID, Face ID, Windows Hello, a security key, or your phone | Generate an enrollment code on the device, then register in BeaconInfra. |
| **Passphrase (fallback)** | A separate remote-access passphrase | Set it on the device; enter it when unlocking in the browser. |
| **Authenticator app (optional extra factor)** | A six-digit, time-based code | Scan a QR code from the device into your authenticator app. Required alongside either primary factor once enrolled. |

The fingerprint reader or face recognition belongs to the **laptop or phone you
browse from**. The machine running Beacon does not need biometric hardware. Your
operating system controls verification and may offer its device PIN or password
instead. This setup protects device access; it is separate from signing in to
your BeaconInfra account.

Run the commands below **on the machine running Beacon, as the same user that
runs its service**. If you use a custom `BEACON_HOME`, use that same setting.
Start by checking which factors are configured:

```bash
beacon remote-access status
```

### Set up a passkey for biometric verification

1. On the machine running Beacon, generate a one-time enrollment code:

   ```bash
   beacon remote-access add-passkey
   ```

2. On your laptop or phone, open **BeaconInfra → Remote Access → Secure unlock →
   Add a passkey**. Select the device running Beacon, enter its enrollment code,
   and optionally give the passkey a label such as “My laptop”. The code expires
   after a few minutes and can only be used once. The agent must be running and
   connected to BeaconInfra during enrollment.
3. Complete your browser's passkey creation prompt using your preferred
   authenticator. This enrolls a new passkey; clicking **Use passkey** in an
   unlock dialog does not enroll one.
4. After enrollment, verify it and restart the running agent:

   ```bash
   beacon remote-access list-passkeys
   beacon restart
   ```

5. When opening a remote terminal or requesting a tunnel connection in
   BeaconInfra, choose **Use passkey** and complete the biometric or PIN prompt.

Beacon stores the passkey's public key and registration metadata locally. The
private key stays with your authenticator or passkey provider; your biometric
data is not sent to Beacon or BeaconInfra. Enrollment is authorized by the
one-time code generated on your device.

### Set up a passphrase fallback

You can keep a passphrase alongside your passkey, or use a passphrase on its own:

```bash
beacon remote-access set-passphrase
beacon restart
```

The command prompts twice without echoing the passphrase. In the browser,
select **Use passphrase instead**. The browser derives a one-time proof from
the passphrase, and the agent verifies it; the passphrase is not sent to the
relay.

### Add an authenticator-app code

This is an **additional factor**, separate from biometric passkey verification.
A passphrase or passkey must already be configured.

```bash
beacon remote-access setup-oob
```

The command prints a **QR code in your terminal**, plus a manual setup secret.
Scan the QR code using your authenticator app, or add the secret manually. Save
the entry before restarting Beacon:

```bash
beacon restart
beacon remote-access status
```

Now enter the current six-digit code in **Authenticator code** whenever you
unlock with either your passkey or passphrase. This code is generated by your
authenticator app and verified on the Beacon device.

Authenticator-app enrollment is currently **CLI-only**; the dashboard does not
provide a QR-code setup wizard. The unlock dialog shows the code field even
when no app is enrolled: **leave it blank if `remote-access status` says
“Out-of-band: not set”**. If you only configured a passphrase, choose
**Use passphrase instead** and leave the code blank.

Running `setup-oob` again **replaces the existing authenticator secret**. It
enables the new factor immediately in the saved configuration, without asking
you to confirm a code first. Do not rerun it just to inspect the setup. Treat
the QR code and manual secret as credentials; do not post them in logs or
support requests.

### Open protected tunnels and storage

When a remote-access factor is configured, enabled tunnels **do not auto-start**.
`beacon tunnel enable` enables their configuration; it does not unlock them.

In BeaconInfra, open the device's **Tunnels** section and choose **Request
reconnect** for the tunnel you want. Complete the passkey or passphrase prompt,
including an authenticator-app code if enrolled. Allow the next agent heartbeat
to deliver the request; use the interval shown in the agent logs.

For browser file access, configure a tunnel named exactly `storage` pointing at
Beacon's dashboard port (the default is `9100`):

```bash
beacon storage add files --root /srv/shared-files --name "My files"
beacon tunnel add storage --port 9100
beacon restart
```

Replace `/srv/shared-files` with an existing directory readable by Beacon. If
the `storage` tunnel is already configured correctly, skip adding it again.
Request and unlock the **storage** tunnel in the dashboard, wait for it to
connect, then open **Storage Shares → Browse**. The current storage browser does
not initiate the unlock/connect flow itself. Seeing a share in the dashboard
confirms its configuration was reported, not that its tunnel is connected.

Restarting Beacon stops tunnels and clears in-memory unlock grants. After a
restart, request and unlock each protected tunnel again before browsing its
service or files.

### Manage factors and recover local access

Inspect the configuration without printing secrets:

```bash
beacon remote-access status
beacon remote-access list-passkeys
```

To remove a factor, run the relevant command locally on the Beacon device, then
restart the agent:

```bash
beacon remote-access remove-passkey <id-or-label>
beacon remote-access clear-passphrase
beacon remote-access remove-oob
beacon restart
```

Each removal preserves the other factors. For example, if you lost access to
your authenticator app, `remove-oob` removes that extra requirement while
keeping your passphrase and passkeys. Do not run every removal command unless
you intend to remove every factor.

`beacon remote-access clear` is a local recovery command that removes **all**
remote-access factors. After restarting, device-side verification is disabled
and enabled tunnels can auto-start. Use it only when you intend to disable the
gate, then enroll your replacement factors.

---

## ⚡ Quick Start

### 1. Install

```bash
curl -fsSL https://raw.githubusercontent.com/Bajusz15/beacon/main/scripts/install.sh | bash
```

One static binary, no runtime dependencies. Builds for Linux (AMD64, ARM64, ARMv7) and macOS.

### 2. Run it

```bash
beacon service install
```

Installs Beacon as a background service (systemd) that starts on boot and auto-restarts. The dashboard is at `http://localhost:9100`; system metrics and project health run locally, no account needed. Operate it with `beacon logs`, `beacon status`, and `beacon restart`.

> Not on Linux/systemd (e.g. macOS)? Run `beacon start` to run it directly instead.

### 3. (Optional) Connect to BeaconInfra

```bash
beacon cloud login --api-key bci_live_xxxxxxxx
beacon restart
```

The first heartbeat registers your device automatically. To disconnect: `beacon cloud logout`. Beacon makes zero outbound calls without an API key.

---

## 🛠️ Set up a project

Beacon manages your apps end-to-end: clone from Git or pull from Docker, run your deploy command, poll for updates, health check, and tail logs. Each project runs as its own isolated process — one crash doesn't affect others.

### Interactive

```bash
beacon bootstrap myapp
```

The wizard asks for deployment type (Git or Docker), repo URL, tokens, and deploy command. It creates a systemd service and kicks off the first deployment.

### From a config file

```bash
beacon bootstrap myapp -f bootstrap.yml
```

**Git:**

```yaml
deployment_type: "git"
repo_url: "https://github.com/you/myapp.git"
git_token: "ghp_xxxxxxxxxxxx"
local_path: "$HOME/beacon/myapp"
deploy_command: "./scripts/deploy.sh"
poll_interval: "60s"
```

**Docker:**

```yaml
deployment_type: "docker"
local_path: "$HOME/beacon/myapp"
poll_interval: "60s"
docker_images:
  - image: "ghcr.io/you/web-app"
    token: "ghp_xxxxxxxxxxxx"
    deploy_command: "docker compose up -d"
    docker_compose_files:
      - "docker-compose.yml"
```

Beacon talks to the registry API, detects the newest tag, pulls it, and runs your command. Supports Docker Hub, GHCR, and any Registry v2-compatible registry. Multiple images in one project are tracked independently — only the changed image redeploys.

### Project secrets

Beacon can store encrypted deploy-time secrets per project and environment:

```bash
beacon secrets set API_TOKEN --project myapp --env prod
beacon secrets list --project myapp --env prod
beacon secrets list --reveal --project myapp --env prod
beacon secrets export --reveal --project myapp --env prod
```

Secrets live under `~/.beacon/secrets/<project>/<env>.enc` and are encrypted with a local AES-256-GCM machine key at `~/.beacon/secrets/key`. The key is created automatically with `0600` permissions.

During deploys, Beacon starts from the process environment, loads the existing `BEACON_SECURE_ENV_PATH` `.env` file when configured, then overlays Beacon secrets. That means Beacon secrets override matching `.env` values. Set `BEACON_PROJECT_ENV=prod` in the project env file to select an environment; if it is not set, Beacon uses `default`.

Values are not printed unless you pass `--reveal`. Use `list` for key names only, `list --reveal` to inspect all values, and `export --reveal --format env|json` for scripts.

Security note: the machine key is stored locally next to the encrypted secret files. This protects against accidental commits, backup leakage, and casual inspection; it is not meant to protect against someone who can already read your Beacon home directory.

### Health checks

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

### Alerts

```yaml
# ~/.beacon/config/projects/myapp/alerts.yml
channels:
  - name: slack
    type: webhook
    url: "$WEBHOOK_URL"

routing:
  - severity: critical
    channels: [slack]
  - severity: warning
    channels: [slack]
    quiet_hours:
      start: "23:00"
      end: "07:00"
      timezone: "Europe/Budapest"
```

Test it: `beacon alerts test --project myapp --severity critical`

---

## 🖥️ Remote terminal

Open a shell on your device from the BeaconInfra dashboard — no SSH, no VPN, no open ports.

The agent picks up a `terminal_open` command via heartbeat, dials back to the cloud over WebSocket, and spawns a local PTY shell. Browser ↔ Cloud ↔ Agent, end-to-end. Sessions auto-expire after 15 minutes or 5 minutes idle.

Security: one-time tokens per session (SHA-256 hashed, server stores only the hash), shell restricted to a known allow-list (`bash`, `zsh`, `sh`, `fish`, etc.), runs as the Beacon agent's OS user.

---

## 🔒 WireGuard VPN

Turn any Beacon device into a peer-to-peer WireGuard exit node. Your traffic flows directly between devices — BeaconInfra only coordinates the key exchange and endpoint discovery.

```bash
# Home device (exit node — needs one port-forwarded UDP port)
beacon vpn enable --listen-port 51820

# Laptop (anywhere)
beacon vpn use my-home-pi
```

Use case: you're on airport WiFi and want to route through your home connection. No subscription, no third-party relay, no trust required. WireGuard is cryptographically silent — port scanners can't even tell it's listening.

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
| **Keep deploy secrets out of `.env` files** | `beacon secrets set API_TOKEN --project myapp --env prod`. Beacon injects them after `.env` values during deploy. |
| **Access Home Assistant, Grafana, or any local service remotely** (with a BeaconInfra account — authenticated, no port-forwarding needed) | `beacon tunnel add homeassistant --port 8123` |
| **Run several tunnels at once** | `beacon tunnel list` / `beacon tunnel enable` / `beacon tunnel disable` |
| **Route traffic through your home network from your laptop** | `beacon vpn enable` on the exit node, `beacon vpn use <device>` on the client. Peer-to-peer WireGuard. |
| **Query Beacon from Cursor or Claude Desktop** | `beacon mcp serve` — see [docs/MCP.md](./docs/MCP.md) |
| **Monitor a Kubernetes cluster** | `beacon source add` with your kubeconfig. |
| **Manage your project list** | `beacon projects list`, `beacon projects add`, `beacon projects status myapp` |

### With a BeaconInfra account (optional)

A free BeaconInfra account adds a hosted dashboard and remote access on top of everything above. Your device keeps running locally — the cloud just gives you somewhere to see it all from a browser, including from your phone.

Turn it on with `beacon cloud login --api-key bci_live_…`. Turn it off any time with `beacon cloud logout`.

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
| **Open a remote terminal session** | Click "Open terminal" on a device in the dashboard. The cloud relays a shell session (PTY) between your browser and the Beacon agent — no SSH port, no VPN needed. Sessions auto-expire after 15 minutes. Enroll a [remote-access passkey or passphrase](#-securing-remote-access) to require a device-verified second factor before any session opens. |
| **Route traffic through your home network** | `beacon vpn enable` on your home device + `beacon vpn use my-pi` on your laptop. WireGuard peer exchange happens via BeaconInfra; the actual traffic is peer-to-peer. For client-only machines, use the lightweight `beacon-vpn` binary. |

### 🔐 What we don't see

Even with BeaconInfra enabled, some things stay on your device and never touch the cloud:

- Your **source code** and **deploy scripts** — the cloud only sends a "deploy now" signal; your device runs the script.
- Your **tokens** (Git, Docker, webhooks) — encrypted locally by `beacon keys`.
- Your **application secrets** (database passwords, API keys loaded via `BEACON_SECURE_ENV_PATH` or `beacon secrets`) — Beacon hands them to your app at deploy time and nothing else.
- **Raw log files** — only the lines you explicitly configured as `log_sources` are forwarded. Everything else stays on disk.
- The **local dashboard** at port 9100 — it keeps working offline, BeaconInfra account or not.

If you change your mind, `beacon cloud logout` stops all outbound reporting on the next heartbeat. There's nothing to delete from a control panel because there's no persistent account state beyond what you chose to send.

---

## 🏗️ Architecture

`beacon start` runs one orchestrator process per device (the "master"). It collects system metrics, serves the local dashboard, sends heartbeats, and supervises everything else. It's stateless per project — it doesn't know about Docker or systemd.

```
┌──────────────────────────────────────────────────────┐
│             BeaconInfra Cloud (optional)              │
│  heartbeats, commands, tunnel proxy, terminal relay   │
└──────────┬───────────────────────────┬───────────────┘
           │ HTTPS                     │ WebSocket
┌──────────┴───────────────────────────┴───────────────┐
│                  beacon start                        │
│                                                       │
│  One per device. System metrics, local dashboard,     │
│  heartbeats, project supervision, tunnel + VPN mgmt.  │
└──┬──────────────┬──────────────┬──────────┬──────────┘
   │ IPC          │ IPC          │ goroutine │ WireGuard
┌──┴───────────┐ ┌┴────────────┐ ┌┴─────────┐ ┌┴─────────┐
│ project agent│ │project agent│ │ tunnels   │ │ VPN      │
│ myapp        │ │ blog        │ │ HA  :8123 │ │ beacon0  │
│ health checks│ │ health check│ │ NC  :8080 │ │ 51820/UDP│
│ log tailing  │ │ log tailing │ │ (WS proxy)│ │          │
└──────────────┘ └─────────────┘ └───────────┘ └──────────┘
```

Projects are isolated: one crash doesn't affect others. The master auto-restarts failed projects with exponential backoff. Tunnels run as lightweight goroutines inside the master, connecting outbound to the cloud via WebSocket so local services are accessible without opening ports.

---

## ⌨️ Commands

| Command | Purpose |
|---------|---------|
| `beacon service install\|uninstall\|status` | Run Beacon as a background service (systemd) |
| `beacon logs` | Show the agent logs (`-f` to follow) |
| `beacon status` | Health view (`--json`, `--watch`, `--no-color`) |
| `beacon restart` | Restart the agent |
| `beacon cloud login` / `logout` | Enable/disable cloud |
| `beacon bootstrap <name>` | Set up a project (interactive or `-f config.yml`) |
| `beacon secrets set\|get\|list\|export\|remove` | Local encrypted deploy secrets |
| `beacon tunnel add\|list\|enable\|disable` | Reverse tunnels for remote access |
| `beacon remote-access add-passkey\|list-passkeys\|remove-passkey` | Enroll/manage passkeys (preferred) that gate remote terminal/tunnel sessions |
| `beacon remote-access setup-oob\|remove-oob` | Enroll/remove an authenticator-app (TOTP) out-of-band factor |
| `beacon remote-access set-passphrase\|clear-passphrase\|status\|clear` | Device-verified passphrase (fallback), gate status, and reset |
| `beacon vpn enable\|use\|disable\|status` | WireGuard VPN |
| `beacon projects list\|add\|remove\|status` | Project management |
| `beacon proxmox list\|status` | On a Proxmox host: list/oversee its VMs & containers (alias: `overseer`) |
| `beacon alerts init\|test\|status` | Alert routing |
| `beacon keys list\|add\|rotate\|delete` | Encrypted token store |
| `beacon mcp serve` | MCP server for Cursor / Claude Desktop |
| `beacon config show` | Show resolved paths and identity |
| `beacon update` | Self-update to latest release |

---

## 🔧 Run as a service

```bash
beacon service install
```

Installs and starts a systemd service that runs the agent, starts on boot, and auto-restarts — a **system** service when run as root, otherwise a **per-user** service. Remove it with `beacon service uninstall`, check it with `beacon service status`.

For per-user services, installation also enables systemd **lingering** so Beacon starts before login and keeps running after you disconnect. This applies to services created by `beacon bootstrap` too. If permissions prevent enabling lingering, the installer prints a `sudo loginctl enable-linger` command to run before retrying. To repair an existing user service, run `beacon service install` again. Uninstalling Beacon leaves lingering enabled because other user services may depend on it.

> On macOS or a host without systemd, run `beacon start` directly (macOS uses launchd).

---

## 🩺 Troubleshooting

```bash
beacon logs -f        # follow the agent logs (systemd journal, or ~/.beacon/master.log)
beacon status         # master + project health
beacon restart        # restart the agent
beacon config show    # resolved paths, device identity, cloud_api_base, reporting on/off
```

**Not sending to the cloud?** Run `beacon config show`: `cloud_reporting_enabled` must be `true` and `cloud_api_base` must point at your cloud. Then watch `beacon logs -f` for the heartbeat result (a `401` means a bad API key; a connection error usually means the wrong `cloud_api_base` or a firewall).

The low-level `beacon start` / `init` / `monitor` commands still work for debugging; they're just hidden from `beacon --help` to keep the everyday surface small.

---

## 📚 Documentation

- [docs/MASTER_AGENT.md](./docs/MASTER_AGENT.md) — agent architecture and heartbeats
- [docs/VPN.md](./docs/VPN.md) — WireGuard VPN setup and security model
- [docs/NAS.md](./docs/NAS.md) — planned NAS file gateway architecture
- [docs/LOG_FORWARDING.md](./docs/LOG_FORWARDING.md) — log forwarding
- [docs/KEY_MANAGEMENT.md](./docs/KEY_MANAGEMENT.md) — encrypted key store
- [docs/MCP.md](./docs/MCP.md) — MCP server for editors
- [examples/](./examples/) — bootstrap, monitor, alert configs

---

☕ **[Buy me a coffee](https://buymeacoffee.com/matebajusz)** — if Beacon saves you time.

## License

Apache 2.0 — see [LICENSE](./LICENSE)
