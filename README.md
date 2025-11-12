# Beacon

<img src="./logo.png" alt="Beacon Logo" width="120" height="120">

**Lightweight deployment and monitoring agent for self-hosted IoT devices**

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20-lightgrey)](https://github.com/Bajusz15/beacon/releases)
[![CI](https://github.com/Bajusz15/beacon/workflows/CI/badge.svg)](https://github.com/Bajusz15/beacon/actions)
[![codecov](https://codecov.io/gh/Bajusz15/beacon/branch/main/graph/badge.svg)](https://codecov.io/gh/Bajusz15/beacon)
[![Go Report Card](https://goreportcard.com/badge/github.com/Bajusz15/beacon)](https://goreportcard.com/report/github.com/Bajusz15/beacon)
[![Security](https://github.com/Bajusz15/beacon/workflows/CI/badge.svg?label=security)](https://github.com/Bajusz15/beacon/security)
[![Release](https://github.com/Bajusz15/beacon/workflows/Release/badge.svg)](https://github.com/Bajusz15/beacon/releases)

Beacon polls a Git repository for new tags and deploys code when a new tag appears, while providing comprehensive infrastructure monitoring and log forwarding. Future proof your IoT or self-hosted deployments with automated monitoring and deployment. 

## 🚀 **5-Minute Quick Start**

Get Beacon up and running in minutes:

### 1. Install Beacon
```bash
curl -fsSL https://raw.githubusercontent.com/Bajusz15/beacon/main/scripts/install.sh | bash
```

### 2. Bootstrap Your First Project
```bash
# Interactive setup (recommended for first time)
beacon bootstrap myapp

# Or use a config file for automation
beacon bootstrap myapp -f beacon.bootstrap.example.yml
```

### 3. Start Monitoring
```bash
# Set up monitoring configuration
cp beacon.monitor.example.yml ~/.beacon/config/projects/myapp/monitor.yml

# Start monitoring
beacon monitor -f ~/.beacon/config/projects/myapp/monitor.yml
```

### 4. Enable Auto-Deployment (Optional)
```bash
# Enable systemd service for automatic deployment
systemctl --user daemon-reload
systemctl --user enable beacon@myapp
systemctl --user start beacon@myapp
```

### 5. Test It Works
```bash
# Check status
systemctl --user status beacon@myapp

# View logs
journalctl --user -u beacon@myapp -f
```

**🎉 That's it!** Your project is now being monitored and will automatically deploy when you push new tags to your Git repository.

> **Need help?** Check out the [Troubleshooting](#-troubleshooting) section or [open an issue](https://github.com/Bajusz15/beacon/issues).

## 📊 **CI Status & Quality**

Our CI pipeline ensures code quality and reliability:

| Badge | Description | Status |
|-------|-------------|--------|
| ![CI](https://github.com/Bajusz15/beacon/workflows/CI/badge.svg) | **Tests & Builds** - Runs tests on Go 1.21, 1.22, 1.23 and builds for all platforms | [View Details](https://github.com/Bajusz15/beacon/actions) |
| ![codecov](https://codecov.io/gh/Bajusz15/beacon/branch/main/graph/badge.svg) | **Test Coverage** - Shows percentage of code covered by tests | [View Coverage](https://codecov.io/gh/Bajusz15/beacon) |
| ![Go Report Card](https://goreportcard.com/badge/github.com/Bajusz15/beacon) | **Code Quality** - Go code quality metrics and analysis | [View Report](https://goreportcard.com/report/github.com/Bajusz15/beacon) |
| ![Security](https://github.com/Bajusz15/beacon/workflows/CI/badge.svg?label=security) | **Security Scan** - Automated security vulnerability scanning | [View Security](https://github.com/Bajusz15/beacon/security) |
| ![Release](https://github.com/Bajusz15/beacon/workflows/Release/badge.svg) | **Releases** - Automated releases with cross-platform binaries | [View Releases](https://github.com/Bajusz15/beacon/releases) |

### CI Pipeline Features

- **Multi-version Testing**: Tests on Go 1.21, 1.22, and 1.23
- **Cross-platform Builds**: Linux (ARM, ARM64, AMD64) and macOS (ARM64, AMD64)
- **Security Scanning**: Gosec and govulncheck for vulnerability detection
- **Code Quality**: golangci-lint with comprehensive rules
- **Test Coverage**: Detailed coverage reports with HTML output
- **Automated Releases**: Triggered by Git tags with checksums
- **Dependency Updates**: Weekly automated dependency updates

### How to Read the Badges

- **Green ✅**: All checks passed
- **Red ❌**: Some checks failed (click badge for details)
- **Yellow ⚠️**: Checks are running or pending
- **Gray ⚪**: Checks are disabled or not configured

Click any badge to see detailed results and logs.

## 📚 **Documentation**

- **[LOG_FORWARDING.md](./LOG_FORWARDING.md)** - Complete guide for log forwarding (file, Docker, deploy, command logs)
- **[beacon.monitor.example.yml](./beacon.monitor.example.yml)** - Comprehensive monitoring configuration examples
- **[beacon.bootstrap.example.yml](./beacon.bootstrap.example.yml)** - Bootstrap configuration template for automation
- **[beacon.env.example](./beacon.env.example)** - Environment configuration template
- **[beacon.wizard.example.yml](./beacon.wizard.example.yml)** - Example output from setup-wizard (monitor config)
- **[beacon.wizard.example.env](./beacon.wizard.example.env)** - Example output from setup-wizard (environment variables)

## 📖 **Table of Contents**

- [🚀 5-Minute Quick Start](#-5-minute-quick-start) ⚡ **Start Here**
- [Features](#-features)
- [Perfect For](#-perfect-for) 🎯 **Who Should Use Beacon**
- [How Beacon Works](#-how-beacon-works) 🔍 **Understanding the System**
- [Bootstrap Setup](#-bootstrap-setup) ⭐ **Recommended**
- [Configuration Files](#-configuration-files)
- [Monitoring Configuration](#monitoring-configuration)
- [Log Forwarding](#-log-forwarding) → See [LOG_FORWARDING.md](./LOG_FORWARDING.md)
- [Installation](#-installation)
- [Troubleshooting](#-troubleshooting)

## ✨ **Features**

- **🚀 Auto-Deployment**: Polls Git repos for new tags and automatically deploys latest versions
- **📊 Health Monitoring**: HTTP endpoints, ports, custom commands, and system metrics
- **📋 Log Forwarding**: Files, Docker containers, deployments, and custom commands
- **🔔 Smart Alerting**: Email, webhooks, Discord, Telegram with severity-based routing
- **⚡ Lightweight**: Single binary, minimal dependencies, perfect for IoT devices
- **🔧 Easy Setup**: Interactive bootstrap wizard or config file automation
- **🛡️ Self-Hosted**: No cloud dependencies, complete privacy and control
- **📱 Status Page**: Built-in web interface for monitoring status
- **🔄 Hot Reload**: Update configuration without restarting services
- **🎯 Multi-Project**: Manage multiple projects independently

## 🎯 **Perfect For**

- **🏠 Self-Hosters**: Home labs, personal servers, IoT projects
- **👨‍💻 Developers**: Individual developers and small teams
- **🌱 Entrepreneurs**: Startups and small businesses
- **🎮 Hobbyists**: Raspberry Pi enthusiasts, makers, tinkerers
- **🔒 Privacy-Focused**: Users who want complete control over their infrastructure
- **⚡ Simple Needs**: Those who want powerful monitoring without enterprise complexity

> **Not for**: Large enterprise teams with complex compliance requirements. Beacon is designed for individuals, small teams, and self-hosted environments.

## 🔔 Simple Alert Routing (NEW!)

Beacon now includes a **simple, powerful alert routing system** perfect for self-hosted IoT monitoring and homelab setups:

### Features

- **🎯 Severity-based routing**: Critical, warning, and info alerts with different channels
- **📱 Multiple channels**: Email, Discord, Telegram, Slack, and webhooks
- **⏰ Simple backup notification**: Notify backup person after configurable delay
- **🌙 Quiet hours**: Suppress non-critical alerts during specified hours
- **🔧 Easy configuration**: Clean YAML configuration with sensible defaults
- **🚀 Perfect for**: Self-hosted IoT, homelab infrastructure, small teams, privacy-first monitoring

### Quick Setup

```bash
# Initialize simple alert routing
beacon alerts init

# Test your configuration
beacon alerts test

# View active alerts
beacon alerts status
```

### Configuration Example

```yaml
# Simple alert routing - perfect for self-hosted setups
alert_routing:
  - severity: "critical"
    channels: ["email", "discord"]
    recipients: 
      - "admin@example.com"
      - "#alerts"
    backup_delay: "10m"  # Notify backup after 10 minutes
    backup_recipients:
      - "backup@example.com"
    quiet_hours:
      enabled: false  # Critical alerts always go through
    enabled: true

  - severity: "warning"
    channels: ["discord"]
    recipients:
      - "#alerts"
    backup_delay: "30m"  # Notify backup after 30 minutes
    quiet_hours:
      enabled: true
      start_time: "23:00"
      end_time: "07:00"
      suppress_severities: ["warning"]  # Suppress warnings during quiet hours
    enabled: true

  - severity: "info"
    channels: ["discord"]
    recipients:
      - "#logs"
    quiet_hours:
      enabled: true
      start_time: "23:00"
      end_time: "07:00"
      suppress_severities: ["info"]  # Suppress info alerts during quiet hours
    enabled: true
```

### Alert Channels

Configure your preferred notification channels:

```yaml
alert_channels:
  email:
    smtp_host: "smtp.gmail.com"
    smtp_port: 587
    smtp_user: "${SMTP_USER}"
    smtp_password: "${SMTP_PASSWORD}"
    from: "Beacon Alerts <alerts@example.com>"
    enabled: true

  discord:
    webhook_url: "${DISCORD_WEBHOOK_URL}"
    username: "Beacon Bot"
    enabled: true

  telegram:
    bot_token: "${TELEGRAM_BOT_TOKEN}"
    chat_id: "${TELEGRAM_CHAT_ID}"
    enabled: true
```

### Why Simple Alert Routing?

Unlike complex enterprise escalation systems, Beacon's simple alert routing is designed for:

- **🏠 Self-hosted environments**: No external dependencies or complex setup
- **🔒 Privacy-first**: All alerts stay within your infrastructure
- **⚡ Lightweight**: Minimal resource usage on IoT devices
- **🎯 Focused**: Does exactly what you need without bloat
- **🔧 Easy to understand**: Clear configuration that anyone can modify

## 🧙‍♂️ **Configuration Wizard**

Beacon includes an interactive configuration wizard to help you set up monitoring quickly:

```bash
# Start the configuration wizard
beacon setup-wizard

# Specify custom paths
beacon setup-wizard --config ./my-config.yml --env .env
```

### Wizard Features

- **Device Templates**: Pre-configured setups for Raspberry Pi, web servers, Docker hosts, and databases
- **Interactive Setup**: Step-by-step configuration with helpful prompts
- **Plugin Configuration**: Easy setup for Discord, Telegram, email, and webhook alerts
- **Environment Variables**: Automatic generation of `.env` file with secure credential placeholders
- **Validation**: Configuration validation before saving

### Supported Templates

1. **Raspberry Pi / IoT Device** - SSH service monitoring, system health checks
2. **Web Server / Application** - HTTP endpoint monitoring, health checks, process monitoring
3. **Docker Container Host** - Docker daemon, compose services, container health
4. **Database Server** - PostgreSQL, MySQL, Redis port monitoring, connection tests
5. **Custom Configuration** - Start minimal and add checks manually

### Example Wizard Output

The wizard generates three files:
- **Monitor config** (`beacon.monitor.yml`) - Your monitoring configuration
- **Environment file** (`.env`) - Credentials and tokens (never commit this!)
- **Bootstrap config** (`beacon.bootstrap.yml`) - Generic bootstrap template ready to customize

See example outputs:
- **[beacon.wizard.example.yml](./beacon.wizard.example.yml)** - Example monitor config generated by wizard
- **[beacon.wizard.example.env](./beacon.wizard.example.env)** - Example environment file with placeholders
- **[beacon.bootstrap.example.yml](./beacon.bootstrap.example.yml)** - Similar to the generated bootstrap config (customize and use with `beacon bootstrap myproject -f beacon.bootstrap.yml`)

After running the wizard:
1. Fill in environment variables in `.env`
2. Customize `beacon.bootstrap.yml` with your repo URL and deploy command
3. Run: `beacon bootstrap myproject -f beacon.bootstrap.yml`
4. Run: `beacon monitor -f beacon.monitor.yml`

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
- **External Reporting**: Sends monitoring data to external systems like [BeaconWatch](https://beaconinfra.dev/)
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

# Use configuration file for non-interactive setup
beacon bootstrap myapp -f beacon.bootstrap.example.yml

# With options
beacon bootstrap myapp --force --skip-systemd
```

**Options:**
- `--force, -f` - Force overwrite of existing components
- `--skip-systemd, -s` - Skip systemd service setup
- `--config, -f` - Use configuration file for non-interactive setup

### Bootstrap Configuration File

For automation and testing, you can use a YAML configuration file instead of interactive prompts:

```bash
beacon bootstrap myapp -f beacon.bootstrap.example.yml
```

**Example configuration file** (`beacon.bootstrap.example.yml`):
```yaml
# Project configuration
project_name: "my-awesome-app"
repo_url: "https://github.com/username/my-awesome-app.git"
local_path: "$HOME/beacon/my-awesome-app"
deploy_command: "./scripts/deploy.sh"
poll_interval: "60s"
port: "8080"

# Authentication (choose one or both)
ssh_key_path: "/home/user/.ssh/id_rsa"  # For SSH URLs
git_token: "ghp_xxxxxxxxxxxxxxxxxxxx"  # For HTTPS URLs (GitHub Personal Access Token)

# Security and environment
secure_env_path: "/etc/beacon/my-awesome-app.env"
user: "deploy-user"
working_dir: "$HOME/beacon/my-awesome-app"
use_system_service: false  # Set to true for system-wide service
```

**Benefits of using config files:**
- **Non-interactive**: Perfect for CI/CD and automation
- **Version controlled**: Store configuration in your repository
- **Reproducible**: Consistent setup across environments

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

  # Custom command checks with alert_command
  - name: "Disk Usage"
    type: command
    command: "df -h / | awk 'NR==2 {print $5}' | sed 's/%//'"
    interval: 300s
    alert_command: "if [ $BEACON_CHECK_OUTPUT -gt 90 ]; then echo 'Disk space critical: $BEACON_CHECK_OUTPUT%' | telegram-send --stdin; fi"

  - name: "Nginx Process"
    type: command
    command: "pgrep nginx > /dev/null && echo 'running' || echo 'stopped'"
    interval: 60s
    alert_command: "if [ '$BEACON_CHECK_OUTPUT' = 'stopped' ]; then echo 'Nginx process stopped!' | curl -X POST -H 'Content-Type: application/json' -d '{\"text\":\"Nginx is down!\"}' ${DISCORD_WEBHOOK_URL}; fi"

  - name: "Memory Usage"
    type: command
    command: "free | awk 'NR==2{printf \"%.0f\", $3*100/$2}'"
    interval: 30s
    alert_command: "if [ $BEACON_CHECK_OUTPUT -gt 85 ]; then echo 'Memory usage high: $BEACON_CHECK_OUTPUT%' | mail -s 'Memory Alert' admin@example.com; fi"

# Alert Command Variables
# For command-type checks, alert_command always runs regardless of check status
# Available variables in alert commands:
#   $BEACON_CHECK_NAME     - Name of the check
#   $BEACON_CHECK_TYPE     - Type of check (command, http, port)
#   $BEACON_CHECK_STATUS   - Status (up, down, error)
#   $BEACON_CHECK_OUTPUT   - Command output (for command checks only)
#   $BEACON_CHECK_ERROR    - Error message if any
#   $BEACON_CHECK_DURATION - Check duration in seconds
#   $BEACON_DEVICE_NAME    - Device name
#
# Example alert commands:
#   - Send to Telegram: "echo 'Alert: $BEACON_CHECK_NAME is $BEACON_CHECK_STATUS' | telegram-send --stdin"
#   - Send to Discord: "curl -X POST -H 'Content-Type: application/json' -d '{\"text\":\"$BEACON_CHECK_NAME: $BEACON_CHECK_STATUS\"}' ${DISCORD_WEBHOOK_URL}"
#   - Send email: "echo 'Check $BEACON_CHECK_NAME is $BEACON_CHECK_STATUS' | mail -s 'Beacon Alert' admin@example.com"
#   - Log to syslog: "logger -p local0.err 'Beacon Alert: $BEACON_CHECK_NAME is $BEACON_CHECK_STATUS'"

# System metrics
system_metrics:
  enabled: true
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

