# Beacon Master Agent

The **Master Agent** provides machine-wide cloud reporting for Beacon. It runs independently of any project and sends heartbeats to BeaconInfra (or a self-hosted API-compatible backend), ensuring your device is always visible in the cloud dashboard.

## Overview

Beacon has two complementary reporting mechanisms:

| Component | Scope | Purpose |
|-----------|-------|---------|
| `beacon master` | Machine-wide | Device heartbeats, machine health |
| `beacon monitor` | Per-project | Project health checks, logs, metrics |

The master agent is ideal when you want:
- A single heartbeat for the entire machine (not tied to any project)
- Device visibility even when no projects are running
- Separation of machine health from project health

## Quick Start

### 1. Initialize Identity

First, set up your cloud identity (no network requests are made):

```bash
# Set environment variables
export BEACON_API_KEY="usr_your_api_key"    # From BeaconInfra UI
export BEACON_CLOUD_URL="https://app.beaconinfra.dev/api"

# Initialize (writes ~/.beacon/config.yaml)
beacon init --name my-homelab-server
```

Or provide all values as flags:

```bash
beacon init \
  --api-key "usr_your_api_key" \
  --cloud-url "https://app.beaconinfra.dev/api" \
  --name my-homelab-server
```

### 2. Start the Master Agent

```bash
# Run in foreground (for testing)
beacon master

# Or use systemd (recommended)
systemctl --user start beacon-master.service
```

### 3. Verify It's Working

Check the systemd service status:

```bash
systemctl --user status beacon-master.service
journalctl --user -u beacon-master.service -f
```

Your device should appear in the BeaconInfra dashboard.

## Configuration

### ~/.beacon/config.yaml

The `beacon init` command creates this file:

```yaml
api_key: usr_your_api_key
device_name: my-homelab-server
cloud_url: https://app.beaconinfra.dev/api
heartbeat_interval: 30        # seconds
cloud_reporting_enabled: true
device_id: ""                 # populated after first successful heartbeat
```

| Field | Description |
|-------|-------------|
| `api_key` | User API key from BeaconInfra (starts with `usr_`) |
| `device_name` | Human-readable name for this device |
| `cloud_url` | API base URL (include `/api` suffix) |
| `heartbeat_interval` | Seconds between heartbeats (default: 30) |
| `cloud_reporting_enabled` | Set to `false` to disable heartbeats |
| `device_id` | Auto-populated UUID after first heartbeat |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `BEACON_API_KEY` | User API key (alternative to config file) |
| `BEACON_CLOUD_URL` | Cloud API URL (alternative to config file) |
| `BEACON_DEVICE_NAME` | Device name (alternative to config file) |

## Systemd Service

### Automatic Installation

When you run `beacon bootstrap`, it automatically:
1. Creates `~/.beacon/config.yaml` if it doesn't exist
2. Installs `beacon-master.service` as a user systemd unit
3. Enables and starts the service

### Manual Installation

If you need to install manually:

```bash
# Create the service file
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

# Enable and start
systemctl --user daemon-reload
systemctl --user enable beacon-master.service
systemctl --user start beacon-master.service
```

### Service Management

```bash
# Check status
systemctl --user status beacon-master.service

# View logs
journalctl --user -u beacon-master.service -f

# Restart after config changes
systemctl --user restart beacon-master.service

# Stop
systemctl --user stop beacon-master.service
```

## Heartbeat Payload

The master agent sends the following data to `POST /agent/heartbeat`:

```json
{
  "hostname": "my-server",
  "ip_address": "192.168.1.100",
  "tags": ["beacon-master"],
  "agent_version": "1.0.0",
  "device_name": "my-homelab-server",
  "os": "linux",
  "arch": "amd64",
  "metadata": {
    "role": "beacon-master"
  }
}
```

The server responds with:

```json
{
  "device_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

The `device_id` is cached in `~/.beacon/config.yaml` for subsequent requests.

## Master vs Monitor Heartbeats

You can run both `beacon master` and `beacon monitor` with heartbeats enabled:

| Scenario | master | monitor heartbeat | Use Case |
|----------|--------|-------------------|----------|
| Machine only | ✓ | - | Device visibility, no project monitoring |
| Project only | - | ✓ | Project health includes device heartbeat |
| Both | ✓ | ✓ | Separate machine/project health reporting |

### When to Use Both

- **Large deployments**: Machine health separate from application health
- **Multiple projects**: One machine heartbeat + multiple project monitors
- **Redundancy**: Backup heartbeat if monitor stops

### When Monitor Heartbeat is Enough

For most single-project deployments, enabling `report.heartbeat.enabled: true` in your `monitor.yml` is sufficient. The monitor sends heartbeats with system metrics.

## Authentication Flow

The authentication priority for API requests:

1. **UserConfig** (`~/.beacon/config.yaml`): `api_key` field
2. **Agent Identity** (`~/.beacon/config/agent.yml`): `device_token` field (legacy)
3. **Monitor config**: `report.token` or `report.token_name` fields

The `beacon init` command sets up option #1, which is the recommended approach.

## Troubleshooting

### Heartbeat Not Sending

Check if cloud reporting is enabled:

```bash
cat ~/.beacon/config.yaml | grep cloud_reporting_enabled
```

Verify API key and cloud URL are set:

```bash
cat ~/.beacon/config.yaml | grep -E "api_key|cloud_url"
```

### Authentication Errors

If you see `401 Unauthorized`:

1. Verify your API key is correct
2. Check the cloud URL includes `/api` suffix
3. Ensure the API key is a user key (starts with `usr_`)

### Service Won't Start

Check for configuration errors:

```bash
# Test manually
beacon master

# Check logs
journalctl --user -u beacon-master.service --no-pager -n 50
```

### Device Not Appearing in Dashboard

1. Check heartbeat is being sent (look for log output)
2. Verify network connectivity to the cloud URL
3. Ensure the device name is unique for your account

## Integration with Bootstrap

When you run `beacon bootstrap`:

```bash
beacon bootstrap myproject
```

The bootstrap process:

1. Prompts: "Send this host's health status to Beacon cloud?" (default: yes)
2. Creates `~/.beacon/config.yaml` with `cloud_reporting_enabled`
3. If enabled, installs and starts `beacon-master.service`

You can control this via bootstrap config file:

```yaml
# beacon.bootstrap.yml
cloud_reporting_enabled: true  # or false to skip
```

Or skip interactively with `--skip-systemd`:

```bash
beacon bootstrap myproject --skip-systemd
```

## Legacy: agent.yml

For backward compatibility, Beacon still supports the legacy `~/.beacon/config/agent.yml` file:

```yaml
server_url: https://your-beacon-host.example/api
device_name: homelab-gateway
device_id: ""
device_token: "dtk_..."  # device token (older auth method)
```

The new `~/.beacon/config.yaml` (created by `beacon init`) is preferred. If both exist, `config.yaml` takes precedence.

## Related Documentation

- [README.md](../README.md) - Main documentation
- [LOG_FORWARDING.md](./LOG_FORWARDING.md) - Log forwarding configuration
- [KEY_MANAGEMENT.md](./KEY_MANAGEMENT.md) - API key management
