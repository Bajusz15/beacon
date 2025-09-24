# 📋 Beacon Log Forwarding Guide

Beacon now supports comprehensive log forwarding to Beaconinfra, including file monitoring, Docker logs, deployment logs, and custom command output.

## 🚀 Quick Start

Configure log sources in your `beacon.monitor.yml`:

```yaml
log_sources:
  - name: "App Logs"
    type: file
    enabled: true
    file_path: "/var/log/app.log"
    interval: 60s
```

## 📁 Log Source Types

### 1. **File-Based Logging**
Monitor log files with real-time tailing:

```yaml
- name: "Application Logs"
  type: file
  enabled: true
  file_path: "/var/log/myapp.log"
  follow_file: true        # tail -f behavior
  interval: 60s
  max_lines: 100
  include_patterns:        # Optional regex filters
    - "ERROR"
    - "WARN"
  exclude_patterns:
    - "DEBUG.*noise"
```

### 2. **Docker Log Forwarding**
Monitor specific containers or all running containers:

```yaml
# Specific containers
- name: "Web Stack Logs"
  type: docker
  enabled: true
  containers: ["nginx", "app", "db"]
  interval: 30s
  docker_options: "--since 5m"

# All running containers
- name: "All Docker Logs"
  type: docker
  enabled: true
  all_containers: true
  interval: 60s
  include_patterns:
    - "(?i)(error|fatal|panic)"
```

### 3. **Deploy Log Forwarding**
Capture deployment command output:

```yaml
- name: "Deploy Logs"
  type: deploy
  enabled: true
  deploy_log_file: "/tmp/beacon-deploy.log"
  interval: 60s
  max_lines: 200
```

**Deploy Command Examples:**
```bash
# Docker Compose with logging
DEPLOY_CMD="docker-compose up --build -d 2>&1 | tee /tmp/beacon-deploy.log"

# Custom script with logging  
DEPLOY_CMD="./deploy.sh 2>&1 | tee /tmp/beacon-deploy.log"

# Multi-step deployment
DEPLOY_CMD="{ echo 'Starting...'; git pull; npm install; npm run build; pm2 restart all; echo 'Done.'; } 2>&1 | tee /tmp/beacon-deploy.log"
```

### 4. **Command-Based Logging**
Execute commands periodically to collect logs:

```yaml
- name: "System Errors"
  type: command
  enabled: true
  command: "journalctl --since '10 minutes ago' -p err -n 50"
  interval: 300s

- name: "Failed SSH Attempts"
  type: command
  enabled: true  
  command: "grep 'Failed password' /var/log/auth.log | tail -20"
  interval: 600s
```

## 🔍 Advanced Filtering

### Include/Exclude Patterns
Use regex patterns to filter log content:

```yaml
include_patterns:
  - "ERROR"
  - "FATAL"
  - "\\[CRITICAL\\]"           # Escape special regex chars
  - "(?i)exception"            # Case-insensitive

exclude_patterns:
  - "health.*check"            # Exclude health checks
  - "GET.*\\.(css|js|png)"     # Exclude static assets
```

### Log Level Detection
Beacon automatically detects log levels (error, warning, info, debug) based on content.

### Log Deduplication
Beacon can automatically filter out duplicate log entries to reduce noise and bandwidth usage:

```yaml
- name: "Application Logs"
  type: file
  enabled: true
  file_path: "/var/log/app.log"
  deduplicate: true  # Enable deduplication
  interval: 30s
```

**How it works:**
- Creates a hash based on source, type, container, and content
- Tracks seen logs for 1 hour to prevent duplicates
- Automatically cleans up old hash entries every 6 hours
- Only affects logs from sources with `deduplicate: true`

## 📊 Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `enabled` | Enable this log source | `false` |
| `interval` | Collection frequency | `60s` |
| `max_lines` | Maximum lines per collection | `100` |
| `max_size` | Maximum log size (future) | - |
| `follow_file` | Tail file continuously | `false` |
| `docker_options` | Additional docker logs options | - |
| `include_patterns` | Regex patterns to include | - |
| `exclude_patterns` | Regex patterns to exclude | - |
| `deduplicate` | Enable log deduplication | `false` |

## 🐳 Docker Examples

```yaml
# Monitor specific application containers
- name: "App Stack"
  type: docker
  enabled: true
  containers: ["web", "api", "worker"]
  interval: 30s
  max_lines: 50

# Monitor all containers with error filtering
- name: "Docker Errors"
  type: docker
  enabled: true
  all_containers: true
  interval: 60s
  include_patterns:
    - "(?i)(error|fatal|panic|exception)"
  docker_options: "--since 10m"
```

## 📁 Common File Patterns

```yaml
# Nginx logs
- name: "Nginx Errors"
  type: file
  file_path: "/var/log/nginx/error.log"
  include_patterns: ["\\[error\\]", "\\[crit\\]"]

# Laravel logs  
- name: "Laravel Logs"
  type: file
  file_path: "/var/www/storage/logs/laravel.log"
  include_patterns: ["ERROR", "CRITICAL", "EMERGENCY"]

# PM2 logs
- name: "PM2 Errors"
  type: file
  file_path: "/home/app/.pm2/logs/app-error.log"
```

## 🚀 Deploy Integration

When using beacon for deployments, redirect output to the configured deploy log file:

1. **Set deploy log file in config:**
   ```yaml
   log_sources:
     - name: "Deploy Logs"
       type: deploy
       enabled: true
       deploy_log_file: "/tmp/beacon-deploy.log"
   ```

2. **Use in deploy commands:**
   ```bash
   # Simple redirect
   your-deploy-command 2>&1 | tee /tmp/beacon-deploy.log
   
   # With timestamp
   echo "Deploy started: $(date)" >> /tmp/beacon-deploy.log
   your-deploy-command 2>&1 | tee -a /tmp/beacon-deploy.log
   echo "Deploy finished: $(date)" >> /tmp/beacon-deploy.log
   ```

## 📈 Best Practices

1. **Start Small**: Begin with essential logs (errors, application logs)
2. **Use Filtering**: Avoid noisy logs with include/exclude patterns
3. **Monitor Performance**: High-frequency collection can impact performance
4. **Rotate Logs**: Ensure log files don't grow indefinitely
5. **Test Patterns**: Verify regex patterns work with your log format

## 📋 Full Example

```yaml
device:
  name: "Production Server"
  tags: ["production", "web"]

log_sources:
  # Critical application errors
  - name: "App Errors"
    type: file
    enabled: true
    file_path: "/var/log/app/error.log"
    interval: 30s
    max_lines: 50

  # Docker stack monitoring
  - name: "Container Logs"
    type: docker
    enabled: true
    containers: ["web", "api", "worker", "redis"]
    interval: 60s
    include_patterns: ["ERROR", "FATAL", "PANIC"]

  # System health
  - name: "System Issues"
    type: command
    enabled: true
    command: "journalctl --since '5 minutes ago' -p err -n 30"
    interval: 300s

  # Deployment tracking
  - name: "Deploy Logs"
    type: deploy
    enabled: true
    deploy_log_file: "/tmp/beacon-deploy.log"
    interval: 60s

report:
  send_to: https://beaconinfra.dev/api/agent
  token: YOUR_API_TOKEN
```

This comprehensive log forwarding system gives you full visibility into your infrastructure from within Beaconinfra! 🎯
