# Beacon
Lightweight deployment and reporting agent for self-hosted IoT devices such as Raspberry Pi. It polls a Git repository for new tags and deploys code when a new tag appears.

---

## ✨ Features

- **Deployment**: Polls a Git repo for new tags and automatically deploys latest versions
- **Monitoring**: Comprehensive health checking for HTTP endpoints, ports, and custom commands
- **Flexible Commands**: Executes custom deploy commands (Docker, scripts, etc.)
- **Status Server**: Runs an HTTP status server with Prometheus metrics support
- **Systemd Compatible**: Easy integration with systemd services
- **Minimal Setup**: Lightweight and easy to configure

---

## **Project Structure for Multi-Repo Support**

```
/etc/beacon/
  projects/
    myapp/
      env
    anotherapp/
      env
/home/pi/
  myapp/
  anotherapp/
systemd/
  beacon@.service
```

---

## **Example Project Environment File**

`beacon.env.example` (copy to `/etc/beacon/projects/myapp/env`):
```
# Example Beacon project environment file

# Repository URL (supports both HTTPS and SSH)
BEACON_REPO_URL=git@github.com:yourusername/yourrepo.git
# For HTTPS: BEACON_REPO_URL=https://github.com/yourusername/yourrepo.git

# Path to SSH private key (optional, for SSH URLs)
BEACON_SSH_KEY_PATH=/etc/beacon/id_ed25519

# Personal access token (optional, for HTTPS URLs)
BEACON_GIT_TOKEN=

# Local deployment path
BEACON_LOCAL_PATH=$HOME/beacon/project

# Deploy command to run after update (optional)
# Example: "docker compose up --build -d" or "./install.sh"
BEACON_DEPLOY_CMD=docker compose up --build -d

# Polling interval (e.g., 30s, 1m, 5m)
BEACON_POLL_INTERVAL=60s

# HTTP server port
BEACON_PORT=8080
```

> **Directory Permissions:**
> - The user running Beacon must have write permissions to the deployment directory specified by `BEACON_LOCAL_PATH` (e.g., `/opt/beacon/project`).
> - If you use a system directory like `/opt/beacon/project`, you can grant access with:
>   ```bash
>   sudo chown -R $(whoami) /opt/beacon/project
>   ```
> - Alternatively, set `BEACON_LOCAL_PATH` to a directory in your home folder (e.g., `/home/pi/beacon/project`), which avoids permission issues.

> **Note:**
> - Use `BEACON_DEPLOY_CMD` to specify a shell command to run after each deployment. This can be a Docker Compose command, a shell script, or any command your project needs.
> - For more complex setups, place an `install.sh` or similar script in your repository and set `BEACON_DEPLOY_CMD=./install.sh`.
> - Beacon will log whether the deploy command succeeded or failed.

---

## **Systemd Service Template**

`systemd/beacon@.service`:
```ini
[Unit]
Description=Beacon Agent for %i - Lightweight deployment and reporting for IoT
After=network.target

[Service]
EnvironmentFile=/etc/beacon/projects/%i/env
Type=simple
ExecStart=/usr/local/bin/beacon
# %h/beacon/%i = $HOME/beacon/{myapp}
# or alternativelty use /opt/beacon/project
WorkingDirectory=%h/beacon/%i
Restart=always
RestartSec=5
User=pi

# Logging
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

> **Note:** With systemd, logs are stored in the systemd journal. View them with:
> ```bash
> journalctl -u beacon@myproject -f
> ```

---

## 📊 Monitoring & Health Checks

Beacon includes a powerful monitoring system that can check the health of your services and infrastructure.

### Monitor Command

Start the monitoring system:

```bash
beacon monitor [config-file]
```

If no config file is specified, Beacon will look for `beacon.monitor.yml` in the current directory.

### Configuration

Create a monitoring configuration file (`beacon.monitor.yml`):

```yaml
checks:
  - name: "Homepage"
    type: http
    url: https://example.com/health
    interval: 30s
    expect_status: 200

  - name: "Database"
    type: port
    host: 127.0.0.1
    port: 5432
    interval: 10s

  - name: "Disk Usage"
    type: command
    cmd: "df -h | grep -w '/'"
    interval: 60s

  - name: "Nginx Process"
    type: command
    cmd: "pgrep nginx"
    interval: 30s

report:
  send_to: https://beaconinfra.dev/api/monitor
  token: YOUR_API_TOKEN
  prometheus_metrics: true
  prometheus_port: 9100
```

### Check Types

#### HTTP Checks
- **Type**: `http`
- **Checks**: HTTP endpoints for availability and expected status codes
- **Metrics**: Response time, status code, duration

#### Port Checks
- **Type**: `port`
- **Checks**: TCP port availability on specified hosts
- **Metrics**: Connection success/failure, duration

#### Command Checks
- **Type**: `command`
- **Checks**: Custom shell commands (supports pipes, redirects, etc.)
- **Metrics**: Command success/failure, output capture, duration

### Monitoring Output

The monitor provides real-time health check results:

```
2025/08/26 10:24:51 [Beacon] Starting monitoring system...
2025/08/26 10:24:51 [Beacon] Check (command) Disk usage: (0.01s) - Output: /dev/disk3s1s1 460Gi 10Gi 357Gi 3% 426k 3.7G 0% /, Error: 
2025/08/26 10:24:51 [Beacon] Check (http) Homepage: up (0.20s)
2025/08/26 10:25:21 [Beacon] Check (http) Homepage: up (0.17s)
2025/08/26 10:25:51 [Beacon] Check (http) Homepage: up (0.16s)
```

### Prometheus Metrics

When `prometheus_metrics: true` is enabled, Beacon exposes metrics at `/metrics`:

```prometheus
# Check status (1 = up, 0 = down/error)
beacon_check_status{name="Homepage",type="http"} 1

# Duration in seconds
beacon_check_duration_seconds{name="Homepage",type="http"} 0.234

# Response time for HTTP checks
beacon_check_response_time_seconds{name="Homepage",type="http"} 0.123

# Last check timestamp
beacon_check_last_check_timestamp{name="Homepage",type="http"} 1703123456
```

### External Reporting

Configure Beacon to send check results to external monitoring systems:

```yaml
report:
  send_to: https://your-monitoring-api.com/checks
  token: YOUR_API_TOKEN
```

Beacon will POST JSON results to the specified endpoint with authentication.

### Example Use Cases

- **Web Application**: Monitor homepage, API endpoints, database connectivity
- **Infrastructure**: Check disk usage, process status, service availability
- **IoT Devices**: Monitor sensor readings, network connectivity, system resources
- **Microservices**: Health checks across multiple services and dependencies


---

## 🚀 Installation

### Available Commands

Beacon provides two main commands:

- **`beacon`** - Deployment agent (polls Git repos and deploys code)
- **`beacon monitor`** - Health monitoring system (checks services and infrastructure)

### Quick Install (Recommended)

For most users, simply run:

```bash
curl -fsSL https://raw.githubusercontent.com/Bajusz15/beacon/main/scripts/install.sh | bash
```

This will:
- Detect your system architecture (ARM, ARM64, AMD64)
- Download the latest release from GitHub
- Install to `/usr/local/bin/beacon`
- Set up systemd service template
- Create necessary directories

### Manual Installation

If you prefer to build from source:

1. Build the binary:
   ```bash
   GOOS=linux GOARCH=arm GOARM=7 go build -o beacon ./cmd/beacon
   ```
   For arm64:
   ```bash
   GOOS=linux GOARCH=arm64 go build -o beacon ./cmd/beacon
   ```

2. Copy to your system:
   ```bash
   chmod +x beacon
   sudo cp beacon /usr/local/bin/beacon
   ```

3. Set up systemd:
   ```bash
   sudo cp systemd/beacon@.service /etc/systemd/system/
   sudo systemctl daemon-reload
   ```

> **Note:** The Beacon binary must be executable. If you encounter permission issues, run:
> ```bash
> sudo chmod +x /usr/local/bin/beacon
> ```

### Configuration Files

Beacon uses two types of configuration files:

1. **`beacon.env`** - Environment variables for deployment (see example above)
2. **`beacon.monitor.yml`** - YAML configuration for monitoring (see `beacon.monitor.example.yml`)

Copy the example files and customize them for your environment:
```bash
cp beacon.env.example beacon.env
cp beacon.monitor.example.yml beacon.monitor.yml
```

## ⚙️ Alternative: Run in Background (without systemd)

If you prefer not to use systemd, you can run `beacon` in the background and log output to a file:

```bash
nohup beacon > beacon.log 2>&1 &
```

To stop it later:
```bash
kill $(pgrep beacon)
```

<pre lang="markdown">

### 🧪 Example Run on Raspberry Pi

```bash
pi@raspberrypi:/media/pi/HIKSEMI/applications/beacon-tests/beacon $ beacon
2025/07/11 17:40:54 [Beacon] Agent starting...
Enter the Git repo URL [https://github.com/yourusername/yourrepo.git]: https://github.com/Bajusz15/beacon.git
Enter the local path for the project [$HOME/beacon/project]: /media/pi/HIKSEMI/applications/beacon-tests/test
Enter the port to run on [8080]: 8080
Enter the SSH key path (optional): 
Enter the Git token (optional): ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
2025/07/11 17:41:22 [Beacon] Status server listening on :8080
2025/07/11 17:42:24 [Beacon] New tag found: v0.0.1 (prev: )
2025/07/11 17:42:24 [Beacon] Deploying tag v0.0.1...
2025/07/11 17:42:25 [Beacon] Deployment of tag v0.0.1 complete.
```

### 🧪 Example Monitoring Run

```bash
pi@raspberrypi:~ $ beacon monitor
2025/08/26 10:24:51 [Beacon] Starting monitoring system...
2025/08/26 10:24:51 [Beacon] Check (command) Disk usage: (0.01s) - Output: /dev/disk3s1s1 460Gi 10Gi 357Gi 3% 426k 3.7G 0% /, Error: 
2025/08/26 10:24:51 [Beacon] Check (http) Homepage: up (0.20s)
2025/08/26 10:25:21 [Beacon] Check (http) Homepage: up (0.17s)
2025/08/26 10:25:51 [Beacon] Check (http) Homepage: up (0.16s)
```
</pre>

---

[☕ Buy me a coffee](https://coff.ee/matebajusz)  
If you find Beacon helpful, consider supporting my work!

## 📄 License

Apache 2.0

---

## 🛠 Troubleshooting

### Permission denied when running `beacon`
Make sure the binary is executable:
```bash
chmod +x beacon
```

### How do I test deployment manually?
Run `beacon` in interactive mode:
```bash
./beacon
```

### Where are logs stored?
If you're using systemd, check logs with:
```bash
journalctl -u beacon@myproject -f
```

If running manually, redirect logs:
```bash
nohup beacon > beacon.log 2>&1 &
tail -f beacon.log
```

### Deployment command is not executing?
Ensure your command is valid and executable. For scripts:
```bash
chmod +x install.sh
```
And use:
```env
BEACON_DEPLOY_CMD=./install.sh
```

If the command fails, Beacon will log the error and exit code.

### Monitoring checks are failing?
- **HTTP checks**: Verify the URL is accessible and returns expected status codes
- **Port checks**: Ensure the service is running and the port is open
- **Command checks**: Test commands manually first, ensure they work in your shell
- **Shell commands**: Use `sh -c "your command"` for complex commands with pipes/redirects

### Prometheus metrics not showing?
- Check that `prometheus_metrics: true` is set in your config
- Verify the port specified in `prometheus_port` is not blocked by firewall
- Access metrics at `http://your-host:port/metrics`

### Command output is truncated?
- Output is limited to 200 characters by default to keep logs readable
- Full output is still captured in the `CheckResult` and sent to external APIs
- Adjust `maxOutputLength` constant in the code if you need longer logs

