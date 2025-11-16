# Beacon Project Enhancement Implementation Plan

## Phase 1-2: Foundation (GitHub Actions + Plugin System)

### 1. GitHub Actions CI/CD Setup

**Create `.github/workflows/ci.yml`:**

- Go 1.21+ setup with matrix testing (multiple Go versions)
- Run tests: `go test -v -race -coverprofile=coverage.out ./...`
- Build verification for all platforms (Linux ARM/ARM64/AMD64, macOS ARM64/AMD64)
- Upload test coverage to Codecov
- Add workflow status badges to README

**Create `.github/workflows/release.yml`:**

- Trigger on new tags (v*)
- Cross-compile binaries using existing Makefile targets
- Generate SHA256 checksums
- Create GitHub release with artifacts
- Auto-generate changelog from commits

**Files to create:**

- `.github/workflows/ci.yml` - Main CI pipeline
- `.github/workflows/release.yml` - Release automation
- `.github/workflows/codeql.yml` - Security scanning

### 2. Plugin System Architecture

**Core plugin interface** (`internal/plugins/plugin.go`):

```go
type Plugin interface {
    Name() string
    Init(config map[string]interface{}) error
    SendAlert(alert Alert) error
    HealthCheck() error
}

type Alert struct {
    Title string
    Message string
    Severity string // critical, warning, info
    Timestamp time.Time
    Device DeviceConfig
    Check *CheckResult
}
```

**Plugin manager** (`internal/plugins/manager.go`):

- Registry for built-in plugins
- Load plugins from config
- Execute alerts across multiple plugins
- Plugin health checks
- Graceful error handling per plugin

**Configuration in `beacon.monitor.yml`:**

```yaml
plugins:
  - name: discord
    enabled: true
    webhook_url: "${DISCORD_WEBHOOK}"
  - name: telegram
    enabled: true
    bot_token: "${TELEGRAM_BOT_TOKEN}"
    chat_id: "${TELEGRAM_CHAT_ID}"
  - name: email
    enabled: true
    smtp_host: "smtp.gmail.com"
    smtp_port: 587
    smtp_user: "${SMTP_USER}"
    smtp_password: "${SMTP_PASSWORD}"
    from: "beacon@example.com"
    to: ["admin@example.com"]
```

**Built-in plugins to implement:**

- `internal/plugins/discord/discord.go` - Discord webhooks
- `internal/plugins/telegram/telegram.go` - Telegram bot API
- `internal/plugins/email/email.go` - SMTP email
- `internal/plugins/webhook/webhook.go` - Generic webhooks with templates

**Integration with monitor:**

- Update `internal/monitor/monitor.go` to load plugin manager
- Replace/enhance `executeAlertCommand` with plugin system
- Add alert rules and thresholds configuration

### 3. Enhanced Testing Infrastructure

**Add benchmark tests:**

- `internal/monitor/monitor_bench_test.go` - Performance benchmarks
- `internal/plugins/plugins_bench_test.go` - Plugin performance

**Integration test improvements:**

- Add Docker Compose test environment
- Test against real services (PostgreSQL, Redis, nginx)
- E2E test with mock BeaconInfra API

**Update existing tests:**

- Increase coverage for edge cases
- Add table-driven tests where applicable
- Mock external dependencies properly

### 4. Configuration Wizard

**Create `internal/wizard/wizard.go`:**

- Interactive TUI using bubbletea or survey library
- Template selection (Raspberry Pi, Docker, web app, IoT)
- Step-by-step configuration builder
- Validate configuration before saving

**Add `beacon setup-wizard` command:**

- Generate both `beacon.monitor.yml` and `.env` files
- Test configurations before saving
- Provide example values and explanations


## Phase 4: Community Building

### 9. Documentation Improvements

**Quick start guide:**

- Add "5-minute quick start" at top of README
- Step-by-step with screenshots
- Common pitfalls highlighted

**Troubleshooting wizard:**

- Interactive troubleshooting in `TROUBLESHOOTING.md`
- Decision tree for common issues
- FAQ section

**Configuration examples:**

- `examples/raspberry-pi/` - Complete Pi setup
- `examples/docker/` - Docker monitoring
- `examples/homelab/` - Homelab infrastructure
- `examples/production/` - Production web app

### 10. Community Infrastructure

**GitHub Discussions:**

- Enable Discussions on repo
- Categories: Q&A, Ideas, Show and Tell
- Pin important discussions

**Contribution guidelines:**

- `CONTRIBUTING.md` - How to contribute
- `CODE_OF_CONDUCT.md` - Community standards
- Issue templates for bugs and features
- PR template with checklist

### 11. Content & Marketing

**Video tutorials:**

- "Getting Started with Beacon" (YouTube)
- "Self-hosting your monitoring stack"
- "Migrating from commercial tools"

**Blog posts:**

- "Why self-host your monitoring"
- "Beacon + Home Assistant integration"
- "Privacy-first infrastructure monitoring"

**Integration guides:**

- Home Assistant integration
- Prometheus exporter setup
- Docker Compose examples

## Implementation Order

1. **Week 1-2**: GitHub Actions CI/CD + Enhanced testing
2. **Week 3-4**: Plugin system core + Discord/Telegram plugins
3. **Week 4-5**: Email plugin + Webhook plugin + Alert rules
4. **Week 5-6**: Configuration wizard + Better error handling
5. **Week 6-7**: Mobile-friendly status server + Documentation
6. **Week 7-8**: BeaconInfra premium features (SMS, analytics)
7. **Week 8-9**: Team collaboration + Incident management
8. **Week 9-10**: Community building + Content creation

## Success Metrics

- Test coverage > 80%
- CI build time < 5 minutes
- GitHub stars > 500 within 3 months
- Active Discord community > 100 members
- 10+ community plugins within 6 months
- User onboarding time < 10 minutes
- BeaconInfra premium conversion > 5%