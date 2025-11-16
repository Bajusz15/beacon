# Beacon Key Management System

The Beacon agent includes a lightweight key management system for secure handling of API keys and Git tokens. This system provides encrypted storage and seamless integration with monitoring and deployment workflows.

## Overview

The key management system handles two types of credentials:

1. **BeaconWatch API Keys**: For reporting monitoring data to the BeaconWatch cloud dashboard
2. **Git Tokens**: For deployment operations (GitHub, GitLab, Bitbucket)

## Key Features

- 🔐 **Encrypted Storage**: All keys are encrypted using AES-GCM
- 🔄 **Easy Rotation**: Rotate keys without changing configuration files
- 🔑 **Multiple Providers**: Support for BeaconWatch, GitHub, GitLab, Bitbucket
- 🔍 **Audit Trail**: Track when keys were created, used, and rotated
- 🚀 **CLI Management**: Simple command-line interface for key operations
- 🔥 **Hot-Reload**: Configuration changes without service restart

## Quick Start

### 1. Add Your BeaconWatch API Key

```bash
# Add your main BeaconWatch API key
beacon keys add --name "production-key" --key "your-beaconwatch-api-key" --provider "beaconwatch" --description "Production BeaconWatch API key"
```

### 2. Add Git Tokens

```bash
# Add GitHub token
beacon keys git add-token --provider "github" --token "ghp_your-token" --name "github-main"

# Add GitLab token
beacon keys git add-token --provider "gitlab" --token "glpat_your-token" --name "gitlab-main"
```

### 3. List All Keys

```bash
# List all stored keys
beacon keys list

# List only Git tokens
beacon keys git list-tokens
```

### 4. Rotate Keys When Needed

```bash
# Rotate a key
beacon keys rotate --name "production-key" --new-key "new-api-key"
```

## Configuration Integration

### Using Stored Keys in Configuration

Instead of hardcoding API keys in your configuration files, reference stored keys:

```yaml
# beacon.monitor.yml
report:
  send_to: https://beaconwatch.dev/api
  token_name: "production-key"  # References stored key
  prometheus_metrics: true
  prometheus_port: 9100
```

### Key Management Configuration

```yaml
# beacon.monitor.yml
report:
  send_to: "https://beaconwatch.dev/api"
  token_name: "production-key"  # References stored key
  prometheus_metrics: true
  prometheus_port: 9100
```

## CLI Commands Reference

### General Key Management

```bash
# List all stored keys
beacon keys list

# Add a new API key
beacon keys add --name "key-name" --key "actual-key" --provider "beaconwatch" --description "Description"

# Rotate an existing key
beacon keys rotate --name "key-name" --new-key "new-key"

# Delete a key
beacon keys delete --name "key-name"

# Validate a key
beacon keys validate --name "key-name"
```

### Git Token Management

```bash
# Add a Git token
beacon keys git add-token --provider "github" --token "token" --name "token-name"

# List Git tokens
beacon keys git list-tokens

# Rotate a Git token
beacon keys git rotate-token --provider "github" --new-token "new-token" --name "token-name"

# Test a Git token
beacon keys git test-token --provider "github" --name "token-name"
```

## Security Features

### Encryption

- All keys are encrypted using AES-GCM with a master key
- Master key is stored securely in `~/.beacon/.master_key`
- Each key file is individually encrypted

### Access Control

- Key files are stored with 600 permissions (owner read/write only)
- Configuration directory has 700 permissions
- Master key file has 600 permissions

### Key Storage Location

```
~/.beacon/
├── .master_key          # Master encryption key
└── keys/                # Encrypted key files
    ├── production-key.json
    ├── github-main.json
    └── ...
```

## Automatic Key Rotation

### How It Works

1. **Detection**: Beacon detects API key failure (401/403 response)
2. **Backup Attempt**: Automatically tries backup keys
3. **Alert**: If all keys fail, triggers rotation alert
4. **Rotation**: Admin rotates key using CLI
5. **Recovery**: Beacon automatically picks up new key
6. **Continuation**: Monitoring continues without interruption

### Rotation Workflow

```bash
# 1. Beacon detects key failure and logs alert
# 2. Admin rotates the key
beacon keys rotate --name "production-key" --new-key "new-api-key"

# 3. Beacon automatically uses the new key
# 4. Monitoring continues seamlessly
```

## Provider Support

### BeaconWatch API Keys

- **Provider**: `beaconwatch`
- **Purpose**: Reporting monitoring data to BeaconWatch cloud dashboard
- **Format**: BeaconWatch API key format
- **Usage**: Referenced in `report.token_name` configuration

### Git Providers

- **GitHub**: `github` - Personal Access Tokens (PAT)
- **GitLab**: `gitlab` - Personal Access Tokens
- **Bitbucket**: `bitbucket` - App Passwords

## Best Practices

### Key Naming Convention

```bash
# Use descriptive names
beacon keys add --name "production-beaconwatch-key" --key "..." --provider "beaconwatch"
beacon keys add --name "staging-beaconwatch-key" --key "..." --provider "beaconwatch"
beacon keys add --name "github-production" --key "..." --provider "github"
```

### Backup Strategy

```bash
# Always maintain backup keys
beacon keys add --name "production-key-primary" --key "..." --provider "beaconwatch"
beacon keys add --name "production-key-backup" --key "..." --provider "beaconwatch"
beacon keys add --name "production-key-emergency" --key "..." --provider "beaconwatch"
```

### Regular Validation

```bash
# Regularly validate keys
beacon keys validate --name "production-key"
beacon keys git test-token --provider "github" --name "github-main"
```

## Troubleshooting

### Common Issues

1. **Key Not Found**
   ```bash
   # Check if key exists
   beacon keys list
   
   # Verify key name spelling
   beacon keys validate --name "exact-key-name"
   ```

2. **Permission Denied**
   ```bash
   # Check file permissions
   ls -la ~/.beacon/
   ls -la ~/.beacon/keys/
   ```

3. **Encryption Errors**
   ```bash
   # Recreate master key (WARNING: This will make existing keys unreadable)
   rm ~/.beacon/.master_key
   beacon keys add --name "test" --key "test" --provider "beaconwatch"
   ```

### Debug Mode

```bash
# Enable debug logging to see key operations
BEACON_DEBUG=1 beacon keys list
```

## Migration from Hardcoded Keys

### Before (Hardcoded)

```yaml
# beacon.monitor.yml
report:
  send_to: https://beaconwatch.dev/api
  token: "hardcoded-api-key-here"  # ❌ Security risk
```

### After (Key Management)

```bash
# 1. Store the key securely
beacon keys add --name "production-key" --key "hardcoded-api-key-here" --provider "beaconwatch"
```

```yaml
# 2. Reference the stored key
# beacon.monitor.yml
report:
  send_to: https://beaconwatch.dev/api
  token_name: "production-key"  # ✅ Secure reference
```

## Integration Examples

### Docker Environment

```bash
# Add keys for Docker environment
beacon keys add --name "docker-beaconwatch" --key "..." --provider "beaconwatch" --description "Docker environment key"
beacon keys git add-token --provider "github" --token "..." --name "docker-github"
```

### IoT Device

```bash
# Add keys for IoT device
beacon keys add --name "iot-beaconwatch" --key "..." --provider "beaconwatch" --description "IoT device key"
```

### Multi-Environment Setup

```bash
# Production
beacon keys add --name "prod-beaconwatch" --key "..." --provider "beaconwatch"
beacon keys add --name "prod-github" --key "..." --provider "github"

# Staging
beacon keys add --name "staging-beaconwatch" --key "..." --provider "beaconwatch"
beacon keys add --name "staging-github" --key "..." --provider "github"

# Development
beacon keys add --name "dev-beaconwatch" --key "..." --provider "beaconwatch"
beacon keys add --name "dev-github" --key "..." --provider "github"
```

This key management system provides enterprise-grade security for your Beacon deployments while maintaining simplicity and ease of use.
