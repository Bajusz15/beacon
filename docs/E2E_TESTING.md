# E2E Testing Guide

## Overview

The Beacon E2E (End-to-End) tests run in a Docker container with a mock Git HTTP server. They cover:

1. **Canonical local-first flow** — `beacon init`, `beacon config show`, and verification of `~/.beacon/config.yaml` (no API key).
2. **Cloud credentials** — `beacon cloud login` (non-interactive flags in CI), then `beacon cloud logout`; asserts `cloud_reporting_enabled` toggles and API key is cleared.
3. **Legacy wizard path** — `beacon setup-wizard` with expect (optional; requires `expect`).
4. **Bootstrap → deploy → monitor** — config-file bootstrap, clone, tag polling, and monitor parsing.

The primary user journey documented in the README is **local master → projects → optional cloud**; the wizard remains an optional helper for generating monitor YAML.

## What is Expect-Based Wizard Input Simulation?

**Expect** is a tool that automates interactive programs by:
- Spawning a process (like `beacon setup-wizard`)
- Watching for specific prompts/patterns
- Sending responses automatically
- Handling timeouts and errors

This allows us to test the wizard non-interactively in CI/CD pipelines.

### Example Expect Script

```expect
spawn beacon setup-wizard
expect "Device name"
send "My Device\r"
expect "Select (1-5):"
send "1\r"
```

The expect script watches for prompts and sends responses, simulating a user typing.

## Test Architecture

### Components

1. **Git HTTP Server** (`scripts/git-http-server.sh`)
   - Simple Go HTTP server that serves Git repositories over HTTP
   - Allows beacon to clone and fetch tags via HTTP (like GitHub/GitLab)
   - Runs on port 8080

2. **E2E Test Script** (`tests/e2e/test-cli.sh`)
   - Orchestrates all test steps
   - Uses expect to automate wizard
   - Verifies bootstrap, deployment, and monitoring

3. **Docker Container** (`tests/e2e/Dockerfile`)
   - Contains all dependencies (git, expect, go)
   - Builds beacon binary
   - Runs tests in isolated environment

## Test Flow

1. **Prerequisites Check** - Verify beacon, git, expect are available
2. **Canonical local init** - `beacon init`, `beacon config show`, assert `cloud_reporting_enabled: false` in config
3. **Cloud login / logout** - `beacon cloud login --api-key …`, then `beacon cloud logout`; assert reporting off and no API key in `beacon config show` (cloud URL is compile-time only)
4. **Create Mock Git Repo** - Set up bare Git repository with initial content
5. **Start Git HTTP Server** - Serve repo over HTTP on port 8080
6. **Test Wizard** - Use expect to automate wizard, verify generated files (optional if expect prompts drift)
7. **Bootstrap Project** - Bootstrap using config file
8. **Verify Bootstrap** - Check env file, config directory created
9. **Verify Repository Cloned** - Confirm repo was cloned correctly
10. **Test Deployment Execution** - Run deploy script manually, verify output
11. **Test Bootstrap Runs Deploy** - Verify bootstrap triggers deploy
12. **Test Secure Env File** - Test secure env file loading
13. **Create New Tag** - Push new tag to repo
14. **Verify Tag Polling** - Test that beacon can fetch new tags
15. **Test Monitoring** - Verify monitor config parsing
16. **Full Workflow Test** - Verify complete integration

## Running E2E Tests

### Locally

```bash
# Build and run
docker compose -f tests/e2e/docker-compose.yml up --build

# Or run script directly (requires expect installed)
./tests/e2e/test-cli.sh
```

### In CI

The tests run automatically in GitHub Actions when:
- Pushed to `main` or `develop` branches
- Pull requests to `main`

## Mock Git Server

The Git HTTP server:
- Serves repositories over HTTP (like GitHub/GitLab)
- Allows `git clone http://localhost:8080/repo.git`
- Supports `git fetch --tags` for polling
- Uses Go's `git upload-pack` commands

### Why Not Use file:// URLs?

While `file://` URLs work for cloning, they don't work well for:
- Tag polling (fetch operations)
- Simulating real-world Git hosting
- Testing HTTP-based authentication

The HTTP server provides a more realistic test environment.

## Test Coverage

### ✅ Wizard Tests
- [x] Wizard generates `beacon.monitor.yml`
- [x] Wizard generates `.env` file
- [x] Wizard generates `beacon.bootstrap.yml`
- [x] All files contain expected content

### ✅ Bootstrap Tests
- [x] Bootstrap creates project structure
- [x] Bootstrap creates environment file
- [x] Bootstrap clones repository
- [x] Bootstrap runs deploy command
- [x] Bootstrap with secure env file

### ✅ Deployment Tests
- [x] Repository is cloned correctly
- [x] Deploy script is executable
- [x] Deploy script runs and produces output
- [x] Deploy log file is created

### ✅ Tag Polling Tests
- [x] New tags can be created and pushed
- [x] Tags can be fetched via HTTP
- [x] Latest tag detection works
- [x] Beacon can poll for new tags

### ✅ Monitoring Tests
- [x] Monitor config can be parsed
- [x] Monitor command is available

## Troubleshooting

### Git HTTP Server Not Starting

Check logs:
```bash
cat /tmp/git-server.log
```

Verify port 8080 is available and not blocked.

### Expect Script Failing

The wizard prompts may have changed. Update the expect script in `test-cli.sh` to match current prompts.

### Repository Not Cloning

- Verify Git HTTP server is running
- Check `GIT_HTTP_URL` is correct
- Ensure bare repo exists at expected path

### Deploy Command Not Running

- Check deploy script is executable
- Verify `BEACON_DEPLOY_COMMAND` in env file
- Check deploy.log for errors

## Log Forwarding E2E (Kubernetes)

A separate e2e test verifies that Beacon forwards logs to an HTTP endpoint when running as a sidecar in Kubernetes.

### Flow

1. **Mock HTTP server** runs in the cluster (Deployment + Service). It accepts `POST /agent/logs`, records the request body to stdout, and returns 200.
2. **Test pod** has two containers:
   - **log-writer**: writes a known line (`E2E_LOG_FORWARDING_MARKER_...`) to `/shared/app.log` every 2s (shared emptyDir).
   - **beacon**: runs `beacon monitor -f /config/monitor.yml` with a file log source for `/shared/app.log` and `report.send_to: http://mock-log-server:8080`.
3. Beacon tails the file, batches logs, and sends them to the mock server. The test waits until the mock server’s logs contain the marker.

### Run locally

Requires **kind**, **kubectl**, and **Docker**.

```bash
./tests/k8s/test.sh
```

The script will:

- Build `mock-log-server:e2e` (from `tests/k8s/mock-log-server/`) and `beacon:e2e` (from `tests/e2e/Dockerfile`).
- Create a kind cluster, load images, deploy the mock server and the test pod.
- Poll `kubectl logs deployment/mock-log-server` for `RECEIVED_POST_AGENT_LOGS` containing `E2E_LOG_FORWARDING_MARKER`.
- Delete the kind cluster on exit.

### Manifests

- `tests/k8s/mock-log-server.e2e.yaml` – Service + Deployment for the mock server.
- `tests/k8s/beacon-log-forwarding.e2e.yaml` – ConfigMap (monitor.yml) + Pod (log-writer + beacon sidecar).

## Future Improvements

- [ ] Test systemd service creation and management
- [ ] Test alerting integration
- [x] Test log forwarding (Kubernetes e2e; see above)
- [ ] Test multiple projects
- [ ] Performance/load testing
- [ ] Test error recovery scenarios

