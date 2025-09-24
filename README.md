# Beacon

<img src="./logo.png" alt="Beacon Logo" width="120" height="120">

**Lightweight deployment and monitoring agent for self-hosted IoT devices**

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20-lightgrey)](https://github.com/Bajusz15/beacon/releases)

Beacon polls a Git repository for new tags and deploys code when a new tag appears, while providing comprehensive infrastructure monitoring and log forwarding. Future proof your IoT or self-hosted deployments with automated monitoring and deployment. 

## 📚 **Documentation**

- **[LOG_FORWARDING.md](./LOG_FORWARDING.md)** - Complete guide for log forwarding (file, Docker, deploy, command logs)
- **[beacon.monitor.example.yml](./beacon.monitor.example.yml)** - Comprehensive configuration examples
- **[beacon.env.example](./beacon.env.example)** - Environment configuration template

## 📖 **Table of Contents**

- [Features](#-features)
- [Quick Start](#-quick-start)
- [How Beacon Works](#-how-beacon-works) 🔍 **Understanding the System**
- [Bootstrap Setup](#-bootstrap-setup) ⭐ **Recommended**
- [Configuration Files](#-configuration-files)
- [Monitoring Configuration](#monitoring-configuration)
- [Log Forwarding](#-log-forwarding) → See [LOG_FORWARDING.md](./LOG_FORWARDING.md)
- [Installation](#-installation)
- [Troubleshooting](#-troubleshooting)

## ✨ **Features**

- **🚀 Deployment**: Polls Git repos for new tags and automatically deploys latest versions
- **📊 Monitoring**: Comprehensive health checking for HTTP endpoints, ports, and custom commands
- **📋 Log Forwarding**: Forward logs from files, Docker containers, deployments, and custom commands
- **📈 System Metrics**: Collect and report CPU, memory, disk usage, uptime, and load average
- **⚙️ Flexible Commands**: Executes custom deploy commands (Docker, scripts, etc.)
- **🔧 Status Server**: Runs an HTTP status server with Prometheus metrics support
- **🔄 Systemd Compatible**: Easy integration with systemd services  
- **⚡ Minimal Setup**: Lightweight and easy to configure

## 🚀 Quick Start

The easiest way to get started with Beacon is using the bootstrap command, which sets up everything automatically:

```bash
# Install Beacon
curl -fsSL https://raw.githubusercontent.com/Bajusz15/beacon/main/scripts/install.sh | bash

# Bootstrap a new project (interactive setup)
beacon bootstrap myapp

# Enable and start the service
systemctl --user daemon-reload
systemctl --user enable beacon@myapp
systemctl --user start beacon@myapp

# Check status
systemctl --user status beacon@myapp
```

**Manual Commands** (bootstrap automatically runs deploy, but not monitoring):
```bash
# Run deployment agent
beacon deploy

# Run monitoring
beacon monitor

# Get help
beacon --help
```

## 🔍 **How Beacon Works**

Beacon operates as a lightweight agent with two main functions: **deployment automation** and **monitoring/logging**. Here's how each component works:

### 🚀 **Deployment Agent (`beacon deploy`)**

The deployment agent automatically manages your application deployments:

- **Repository Polling**: Continuously monitors your Git repository for new release tags
- **Automatic Deployment**: When a new tag is detected, automatically pulls the latest code and runs your deploy command
- **First Install**: If no tags are found, deploys the default branch (usually `main` or `master`)
- **Custom Commands**: Executes your specified deploy command (e.g., `docker compose up --build -d`, `./install.sh`)
- **Status Server**: Runs an HTTP server providing deployment status and health information

**Example Workflow:**
1. You push a new tag `v1.2.3` to your repository
2. Beacon detects the new tag within your polling interval
3. Automatically pulls the latest code to your deployment directory
4. Runs your deploy command (e.g., Docker Compose, installation script)
5. Reports success/failure status via HTTP endpoint

### 📊 **Monitoring Agent (`beacon monitor`)**

The monitoring agent provides comprehensive infrastructure monitoring:

- **Health Checks**: Monitors HTTP endpoints, TCP ports, and custom commands
- **System Metrics**: Collects CPU, memory, disk usage, and other system information
- **Log Forwarding**: Captures and forwards logs from files, Docker containers, and commands
- **External Reporting**: Sends monitoring data to external systems like [BeaconInfra](https://beaconinfra.dev/)
- **Prometheus Metrics**: Exposes metrics in Prometheus format for integration with monitoring stacks

**Integration with BeaconInfra Cloud Dashboard:**
- **Centralized Monitoring**: View all your devices and their health status in one dashboard
- **Log Aggregation**: Centralized log viewing and analysis across multiple devices
- **Website Monitoring**: Set up independent HTTP/HTTPS monitoring checks from BeaconInfra's cloud infrastructure
- **Alerting**: Configure email and webhook notifications for failures
- **Historical Data**: Track uptime percentages and performance trends over time

### 🔄 **Command Integration**

**Bootstrap automatically sets up deployment:**
```bash
beacon bootstrap myapp  # Sets up deploy agent with systemd service
```

**Monitoring runs separately:**
```bash
beacon monitor  # Runs monitoring based on beacon.monitor.yml config
```

**Typical Setup:**
1. **Bootstrap** creates deployment automation for your application
2. **Monitor** runs independently to watch system health and forward logs
3. **BeaconInfra Dashboard** provides centralized view of all your devices and monitoring data

This separation allows you to:
- Run deployment automation without monitoring overhead
- Monitor multiple applications from a single device
- Scale monitoring independently from deployment needs
- Use BeaconInfra's cloud infrastructure for external monitoring checks

## ⭐ **Bootstrap Setup**

The `beacon bootstrap` command is the recommended way to set up new projects. It creates all necessary directories, configuration files, and optionally sets up systemd services automatically.

### Basic Bootstrap Usage

```bash
# Interactive setup (recommended)
beacon bootstrap myapp

# With options
beacon bootstrap myapp --force --skip-systemd
```

**Options:**
- `--force, -f` - Force overwrite of existing components
- `--skip-systemd, -s` - Skip systemd service setup

### What Bootstrap Creates

The bootstrap command automatically sets up:

1. **Project Configuration Directory**: `~/.beacon/config/projects/myapp/`
2. **Environment File**: `~/.beacon/config/projects/myapp/env`
3. **Working Directory**: `~/beacon/myapp/`
4. **Local Deployment Path**: Where your Git repository will be cloned
5. **User Systemd Service**: `~/.config/systemd/user/beacon@myapp.service`
What you mose provide:

1. **Secure Environment File**: For storing sensitive deployment variables

### Bootstrap Interactive Prompts

During bootstrap, you'll be prompted for:

- **Project Name**: Unique identifier for your project
- **Git Repository URL**: HTTPS or SSH URL to your repository
- **Local Deployment Path**: Where to clone and deploy your code
- **Deploy Command**: Command to run after deployment (e.g., `docker compose up --build -d`)
- **Polling Interval**: How often to check for new tags (e.g., `60s`, `5m`)
- **HTTP Server Port**: Port for Beacon's status server
- **SSH Key Path**: Optional SSH key for private repositories
- **Git Token**: Optional personal access token for HTTPS repositories
- **Secure Environment File Path**: Optional path for sensitive environment variables

### Example Bootstrap Session

```bash
$ beacon bootstrap myapp
[Beacon] Starting project bootstrap...
[Beacon] This will create the necessary directory structure, configuration files,
[Beacon] and optionally set up systemd services for your beacon project.

Enter project name [myapp]: myapp
Enter Git repository URL [https://github.com/yourusername/yourrepo.git]: https://github.com/myuser/myapp.git
Enter local deployment path [/home/pi/beacon/myapp]: /opt/myapp
Enter deploy command (optional): docker compose up --build -d
Enter polling interval [60s]: 2m
Enter HTTP server port [8080]: 8080
Enter SSH key path (optional): 
Enter Git token (optional): ghp_xxxxxxxxxxxxxxxxxxxx
Enter secure environment file path (optional) [/etc/beacon/myapp.env]: /etc/myapp/.env
Set up systemd service? (Y/n): Y

[Beacon] Created directories:
  - /home/pi/.beacon/config/projects/myapp
  - /opt/myapp
  - /home/pi/.beacon
[Beacon] Created environment file: /home/pi/.beacon/config/projects/myapp/env
[Beacon] Created user systemd service: /home/pi/.config/systemd/user/beacon@myapp.service
[Beacon] Set permissions:
  - Working directory owned by pi
  - Environment file readable by all users
[Beacon] Bootstrap completed successfully!

Next steps:
1. Review configuration: /home/pi/.beacon/config/projects/myapp/env
2. Edit configuration if needed
3. Set up secure environment file: /etc/myapp/.env
   Add your deployment environment variables (API keys, database URLs, etc.)
   Example: sudo nano /etc/myapp/.env
   Set permissions: sudo chmod 600 /etc/myapp/.env
4. Enable and start the user systemd service:
   systemctl --user daemon-reload
   systemctl --user enable beacon@myapp
   systemctl --user start beacon@myapp
   systemctl --user status beacon@myapp
   journalctl --user -u beacon@myapp -f
5. Test deployment by checking the status endpoint: http://localhost:8080/status
```

### Managing Multiple Projects

You can bootstrap multiple projects on the same system:

```bash
beacon bootstrap webapp
beacon bootstrap api
beacon bootstrap anotherapp

# Each project gets its own systemd service
systemctl --user enable beacon@webapp
systemctl --user enable beacon@api
systemctl --user enable beacon@anotherapp
```

### Post-Bootstrap Configuration

After bootstrap, you may want to:

1. **Edit the environment file**:
   ```bash
   nano ~/.beacon/config/projects/myapp/env
   ```

2. **Set up secure environment variables**:
   ```bash
   sudo nano /etc/myapp/.env
   sudo chmod 600 /etc/myapp/.env
   ```

> **Note:** With systemd, logs are stored in the systemd journal. View them with:
> ```bash
> journalctl -u beacon@myproject -f
> ```
3. **Create monitoring configuration**:
   ```bash
   cp beacon.monitor.example.yml ~/.beacon/config/projects/myapp/monitor.yml
   nano ~/.beacon/config/projects/myapp/monitor.yml
   ```


## 📋 **Configuration Files**

Beacon uses two main configuration files:

1. **`beacon.env`** - Environment variables for deployment settings
   - Repository URLs, deploy commands, polling intervals
   - See: [beacon.env.example](./beacon.env.example)

2. **`beacon.monitor.yml`** - YAML configuration for monitoring and log forwarding
   - Health checks, system metrics, log sources, reporting
   - See: [beacon.monitor.example.yml](./beacon.monitor.example.yml) for comprehensive examples
   - See: [LOG_FORWARDING.md](./LOG_FORWARDING.md) for detailed log forwarding setup

### Application Environment Variables

For your application's environment variables (API keys, database URLs, etc.), you should store them in a separate `.env` file that Beacon can access. This keeps sensitive data separate from Beacon's configuration.

**Recommended locations:**
- `/etc/yourproject/.env` (system-wide, requires sudo)
- `~/.config/yourproject/.env` (user-specific)
- `/opt/yourproject/.env` (application-specific)

**Example setup:**
```bash
# Create secure environment file
sudo nano /etc/myapp/.env

# Add your application variables
DATABASE_URL=postgresql://user:pass@localhost:5432/mydb
API_KEY=your-secret-api-key
REDIS_URL=redis://localhost:6379

# Set proper permissions
sudo chmod 600 /etc/myapp/.env
sudo chown $USER:$USER /etc/myapp/.env
```

**Configure Beacon to use it:**
```bash
# In your beacon.env or bootstrap setup
BEACON_SECURE_ENV_PATH=/etc/myapp/.env
```

**Quick Setup (without bootstrap):**
```bash
# Copy example configurations
cp beacon.env.example beacon.env
cp beacon.monitor.example.yml beacon.monitor.yml

# Edit configurations for your environment
nano beacon.env
nano beacon.monitor.yml
```

---

## 📋 **Log Forwarding**

Beacon supports comprehensive log forwarding to external monitoring systems like Beaconinfra. Configure multiple log sources in your `beacon.monitor.yml`:

```yaml
log_sources:
  # File-based log forwarding
  - name: "Application Logs"
    type: file
    enabled: true
    file_path: "/var/log/app.log"
    interval: 60s
    max_lines: 100
    filters:
      - "ERROR"
      - "WARN"

  # Docker container logs
  - name: "Container Logs"
    type: docker
    enabled: true
    containers: ["web", "api", "worker"]
    interval: 30s
    since: "5m"


  # System logs via commands
  - name: "System Errors"
    type: command
    enabled: true
    command: "journalctl --since '5 minutes ago' -p err -n 30"
    interval: 300s

  # Custom log processing
  - name: "Nginx Access Logs"
    type: file
    enabled: true
    file_path: "/var/log/nginx/access.log"
    interval: 30s
    command: "tail -n 50"
```

**Key Features:**
- **Multiple Sources**: Files, Docker containers, commands, deploy logs
- **Filtering**: Filter logs by keywords, severity levels
- **Rate Limiting**: Control log volume with intervals and line limits
- **External Reporting**: Send logs to monitoring systems
- **Real-time Processing**: Stream logs as they're generated

**📖 For complete log forwarding setup, filtering, Docker examples, and deploy integration:**
👉 **[See LOG_FORWARDING.md](./LOG_FORWARDING.md)**

---

### Monitoring Configuration

Create a monitoring configuration file (`beacon.monitor.yml`):

```yaml
# Device identification
device:
  name: "Production Server"
  location: "Homelab"
  tags: ["production", "web-server", "docker"]
  environment: "production"

# Health checks
checks:
  # HTTP endpoint monitoring with custom headers
  - name: "Homepage"
    type: http
    url: https://example.com/health
    interval: 30s
    expect_status: 200
    timeout: 10s
    headers:
      User-Agent: "Beacon-Monitor/1.0"
    alert_command: "curl -X POST https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK -d '{\"text\":\"🚨 Website is down! Check: $BEACON_CHECK_NAME, Error: $BEACON_CHECK_ERROR\"}'"

  # Database connectivity
  - name: "Database"
    type: port
    host: 127.0.0.1
    port: 5432
    interval: 60s
    timeout: 5s
    alert_command: "echo 'Database connection failed' | mail -s 'Alert: DB Down' admin@example.com"

  # Custom command checks
  - name: "Disk Usage"
    type: command
    cmd: "df -h | grep -w '/' | awk '{print $5}' | sed 's/%//'"
    interval: 300s
    expect_output: "85"  # Alert if usage > 85%

  - name: "Nginx Process"
    type: command
    cmd: "pgrep nginx > /dev/null && echo 'running' || echo 'stopped'"
    interval: 60s
    expect_output: "running"

  # System metrics
  - name: "Memory Usage"
    type: command
    cmd: "free | grep Mem | awk '{printf \"%.1f\", $3/$2 * 100.0}'"
    interval: 120s

# Reporting configuration
report:
  send_to: https://beaconinfra.dev/api
  token: YOUR_API_TOKEN
  interval: 60s
  prometheus_metrics: true
  prometheus_port: 9100
  prometheus_path: "/metrics"
```

### Check Types

#### HTTP Checks
- **Type**: `http`
- **Purpose**: Monitor HTTP/HTTPS endpoints for availability and performance
- **Features**: 
  - Custom headers and authentication
  - Expected status codes
  - Response time monitoring
  - SSL certificate validation
- **Metrics**: Response time, status code, duration, SSL expiry

#### Port Checks
- **Type**: `port`
- **Purpose**: Verify TCP port availability on specified hosts
- **Features**:
  - Connection timeout configuration
  - Multiple port ranges
  - Custom connection tests
- **Metrics**: Connection success/failure, duration, connection time

#### Command Checks
- **Type**: `command`
- **Purpose**: Execute custom shell commands for system monitoring
- **Features**:
  - Full shell command support (pipes, redirects, etc.)
  - Output validation with `expect_output`
  - Exit code checking
  - Custom alert commands
- **Metrics**: Command success/failure, output capture, duration, exit code

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

Beacon provides three main commands:

- **`beacon bootstrap`** - Project setup and systemd service creation ⭐ **Recommended**
- **`beacon deploy`** - Deployment agent (polls Git repos for tags and deploys code automatically)
- **`beacon monitor`** - Health monitoring system (runs independently, forwards logs to [BeaconInfra](https://beaconinfra.dev/))

**Command Separation:**
- **Bootstrap** sets up deployment automation (runs `deploy` automatically via systemd)
- **Deploy** runs continuously, polling for new release tags and deploying when found
- **Monitor** runs separately, providing health checks and log forwarding to cloud dashboard

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

3. Set up systemd (optional, bootstrap handles this automatically):
   ```bash
   sudo cp systemd/beacon@.service /etc/systemd/system/
   sudo systemctl daemon-reload
   ```

> **Note:** The Beacon binary must be executable. If you encounter permission issues, run:
> ```bash
> sudo chmod +x /usr/local/bin/beacon
> ```

### Post-Installation Setup

After installation, use the bootstrap command to set up your first project:

```bash
# Set up a new project
beacon bootstrap myapp

# Enable and start the service
systemctl --user daemon-reload
systemctl --user enable beacon@myapp
systemctl --user start beacon@myapp
```


## ⚙️ Alternative: Run in Background (without systemd)

If you prefer not to use systemd, you can run `beacon` in the background and log output to a file:

```bash
nohup beacon monitor > beacon.log 2>&1 &
```

To stop it later:
```bash
kill $(pgrep beacon)
```

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

### Bootstrap command fails
If bootstrap encounters issues:
```bash
# Check if beacon binary exists
which beacon

# Run with verbose output
beacon bootstrap myapp --force

# Skip systemd if having issues
beacon bootstrap myapp --skip-systemd
```

### How do I test deployment manually?
Run `beacon` in interactive mode:
```bash
beacon deploy
```

### Where are logs stored?
If you're using systemd (recommended), check logs with:
```bash
# For user services
journalctl --user -u beacon@myproject -f

# For system services
journalctl -u beacon@myproject -f
```

If running manually, redirect logs:
```bash
nohup beacon deploy > beacon.log 2>&1 &
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

### Service won't start
Check the service status and logs:
```bash
systemctl --user status beacon@myproject
journalctl --user -u beacon@myproject --no-pager
```

Common issues:
- Binary not found: Check `/usr/local/bin/beacon` exists
- Permission issues: Ensure user has access to deployment directory
- Configuration errors: Check environment file syntax

### Command output is truncated?
- Output is limited to 200 characters by default to keep logs readable
- Full output is still captured in the `CheckResult` and sent to external APIs
- Adjust `maxOutputLength` constant in the code if you need longer logs

### Multiple projects not working
Each project needs its own bootstrap setup:
```bash
beacon bootstrap project1
beacon bootstrap project2
beacon bootstrap project3

# Enable all services
systemctl --user enable beacon@project1
systemctl --user enable beacon@project2
systemctl --user enable beacon@project3
```

