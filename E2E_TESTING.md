# E2E Testing Guide

## Overview

The Beacon E2E (End-to-End) tests verify the complete workflow from wizard setup through bootstrap, deployment, and monitoring. All tests run in a Docker container with a mock Git HTTP server.

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

2. **E2E Test Script** (`scripts/test-e2e-cli.sh`)
   - Orchestrates all test steps
   - Uses expect to automate wizard
   - Verifies bootstrap, deployment, and monitoring

3. **Docker Container** (`Dockerfile.e2e`)
   - Contains all dependencies (git, expect, go)
   - Builds beacon binary
   - Runs tests in isolated environment

## Test Flow

1. **Prerequisites Check** - Verify beacon, git, expect are available
2. **Create Mock Git Repo** - Set up bare Git repository with initial content
3. **Start Git HTTP Server** - Serve repo over HTTP on port 8080
4. **Test Wizard** - Use expect to automate wizard, verify all 3 files generated
5. **Bootstrap Project** - Bootstrap using config file
6. **Verify Bootstrap** - Check env file, config directory created
7. **Verify Repository Cloned** - Confirm repo was cloned correctly
8. **Test Deployment Execution** - Run deploy script manually, verify output
9. **Test Bootstrap Runs Deploy** - Verify bootstrap triggers deploy
10. **Test Secure Env File** - Test secure env file loading
11. **Create New Tag** - Push new tag to repo
12. **Verify Tag Polling** - Test that beacon can fetch new tags
13. **Test Monitoring** - Verify monitor config parsing
14. **Full Workflow Test** - Verify complete integration

## Running E2E Tests

### Locally

```bash
# Build and run
docker compose -f docker-compose.e2e.yml up --build

# Or run script directly (requires expect installed)
./scripts/test-e2e-cli.sh
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

The wizard prompts may have changed. Update the expect script in `test-e2e-cli.sh` to match current prompts.

### Repository Not Cloning

- Verify Git HTTP server is running
- Check `GIT_HTTP_URL` is correct
- Ensure bare repo exists at expected path

### Deploy Command Not Running

- Check deploy script is executable
- Verify `BEACON_DEPLOY_COMMAND` in env file
- Check deploy.log for errors

## Future Improvements

- [ ] Test systemd service creation and management
- [ ] Test alerting integration
- [ ] Test log forwarding
- [ ] Test multiple projects
- [ ] Performance/load testing
- [ ] Test error recovery scenarios

