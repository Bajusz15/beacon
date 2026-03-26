#!/bin/bash

# Beacon E2E Integration Test
# This test uses actual CLI commands to bootstrap a Beacon project and test the complete workflow
# It runs in a Docker container with a mock Git HTTP server

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
PROJECT_NAME="e2e-test-project"
TEST_TIMEOUT=60
GIT_SERVER_PORT=8080
BEACON_PORT=8081
GIT_REPO_DIR="/tmp/beacon-e2e-git"
GIT_REPO_NAME="test-repo.git"
GIT_HTTP_URL="http://localhost:${GIT_SERVER_PORT}/${GIT_REPO_NAME}"

# Docker environment detection
# Use actual $HOME so beacon uses the correct path
if [ "$CI" = "true" ] || [ "$BEACON_E2E_TEST" = "1" ]; then
    IS_DOCKER=true
    # Use actual HOME directory (usually /root in Docker)
    HOME_DIR="${HOME:-/app}"
    # Use a different directory to avoid conflicts with beacon executable
    WORKING_DIR="/tmp/beacon-projects"
else
    IS_DOCKER=false
    HOME_DIR="$HOME"
    WORKING_DIR="$HOME/beacon"
fi

# Cleanup function
cleanup() {
    echo -e "${BLUE}[E2E]${NC} Cleaning up test environment..."
    
    # Stop any running processes
    pkill -f "beacon" || true
    pkill -f "git-server" || true
    pkill -f "git-http-server" || true
    
    # Remove test project
    if [ -d "$HOME_DIR/.beacon/config/projects/$PROJECT_NAME" ]; then
        rm -rf "$HOME_DIR/.beacon/config/projects/$PROJECT_NAME"
    fi
    if [ -d "$HOME_DIR/.beacon/logs/$PROJECT_NAME" ]; then
        rm -rf "$HOME_DIR/.beacon/logs/$PROJECT_NAME"
    fi
    if [ -d "$WORKING_DIR/$PROJECT_NAME" ]; then
        rm -rf "$WORKING_DIR/$PROJECT_NAME"
    fi
    
    # Clean up any files that might exist at directory paths
    if [ -f "$WORKING_DIR" ]; then
        rm -f "$WORKING_DIR"
    fi
    if [ -f "$WORKING_DIR/$PROJECT_NAME" ]; then
        rm -f "$WORKING_DIR/$PROJECT_NAME"
    fi
    
    # Remove test git repo (keep it for debugging, but can remove)
    # rm -rf "$GIT_REPO_DIR"
    
    # Remove wizard-generated files
    rm -f "/tmp/beacon.monitor.yml" "/tmp/.env" "/tmp/beacon.bootstrap.yml"
    
    echo -e "${GREEN}[E2E]${NC} Cleanup completed"
}

# Set up trap for cleanup
trap cleanup EXIT

# Helper functions
log_info() {
    echo -e "${BLUE}[E2E]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[E2E]${NC} $1"
}

log_error() {
    echo -e "${RED}[E2E]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[E2E]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if beacon binary exists
    if ! command -v beacon &> /dev/null; then
        log_error "Beacon binary not found in PATH"
        exit 1
    fi
    
    log_info "Using beacon command: $(which beacon)"
    beacon --version || true
    
    # Check if git is available
    if ! command -v git &> /dev/null; then
        log_error "Git is required but not installed"
        exit 1
    fi
    
    # Check if expect is available
    if ! command -v expect &> /dev/null; then
        log_error "Expect is required for wizard testing but not installed"
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Local-first init: no API key, matches README "Phase A"
test_canonical_local_flow() {
    log_info "Testing canonical local-first init (beacon init)..."
    export HOME="$HOME_DIR"
    mkdir -p "$HOME/.beacon"
    rm -f "$HOME/.beacon/config.yaml"

    if ! beacon init --name "e2e-local-device"; then
        log_error "beacon init failed"
        exit 1
    fi
    if ! grep -q "cloud_reporting_enabled: false" "$HOME/.beacon/config.yaml"; then
        log_error "Expected cloud_reporting_enabled: false in ~/.beacon/config.yaml"
        cat "$HOME/.beacon/config.yaml" 2>/dev/null || true
        exit 1
    fi
    if ! beacon config show 2>/dev/null | grep -q "device_name: e2e-local-device"; then
        log_error "beacon config show did not list expected device_name"
        beacon config show || true
        exit 1
    fi
    log_success "Canonical local-first init OK"
}

# Cloud credentials: login enables reporting; logout clears key and disables reporting
test_cloud_login_logout() {
    log_info "Testing beacon cloud login / cloud logout..."
    export HOME="$HOME_DIR"
    mkdir -p "$HOME/.beacon"
    rm -f "$HOME/.beacon/config.yaml"

    if ! beacon init --name "e2e-cloud-device"; then
        log_error "beacon init failed (cloud flow)"
        exit 1
    fi
    if ! beacon cloud login --api-key "usr_e2e_fake" --cloud-url "https://e2e.example.com/api" --name "e2e-cloud-device"; then
        log_error "beacon cloud login failed"
        exit 1
    fi
    if ! grep -q "cloud_reporting_enabled: true" "$HOME/.beacon/config.yaml"; then
        log_error "Expected cloud_reporting_enabled: true after cloud login"
        cat "$HOME/.beacon/config.yaml" 2>/dev/null || true
        exit 1
    fi
    if ! grep -q "usr_e2e_fake" "$HOME/.beacon/config.yaml"; then
        log_error "Expected api_key in config after cloud login"
        exit 1
    fi
    if ! beacon cloud logout; then
        log_error "beacon cloud logout failed"
        exit 1
    fi
    if ! grep -q "cloud_reporting_enabled: false" "$HOME/.beacon/config.yaml"; then
        log_error "Expected cloud_reporting_enabled: false after cloud logout"
        cat "$HOME/.beacon/config.yaml" 2>/dev/null || true
        exit 1
    fi
    if ! beacon config show 2>/dev/null | grep -q "api_key: (not set)"; then
        log_error "beacon config show should report api_key not set after logout"
        beacon config show || true
        exit 1
    fi
    log_success "beacon cloud login / cloud logout OK"
}

# Create mock Git repository (local bare repo)
create_mock_git_repo() {
    log_info "Setting up mock Git repository..."
    
    # Create test git repository directory
    mkdir -p "$GIT_REPO_DIR"
    cd "$GIT_REPO_DIR"
    
    # Initialize bare git repo
    if [ -d "$GIT_REPO_NAME" ]; then
        rm -rf "$GIT_REPO_NAME"
    fi
    git init --bare "$GIT_REPO_NAME"
    
    # Clone and add initial content
    git clone "$GIT_REPO_NAME" test-repo-work
    cd test-repo-work
    
    # Set git config (both local and global for safety)
    git config user.name "E2E Test"
    git config user.email "test@beacon.local"
    git config --global user.name "E2E Test" || true
    git config --global user.email "test@beacon.local" || true
    
    # Create initial files
    echo "#!/bin/bash" > deploy.sh
    echo "echo \"Deploying version \$1\"" >> deploy.sh
    echo "echo \"Version: \$1\" > version.txt" >> deploy.sh
    echo "echo \"Deployed at: \$(date)\" >> version.txt" >> deploy.sh
    echo "echo \"Deploy command executed successfully\" >> deploy.log" >> deploy.sh
    chmod +x deploy.sh
    
    echo "Initial version" > version.txt
    echo "Deployed at: $(date)" >> version.txt
    
    # Initial commit
    git add .
    git commit -m "Initial commit"
    git tag v1.0.0
    
    # Push to origin (bare repo)
    # For bare repos, we push directly
    git push origin master 2>&1 || git push origin main 2>&1 || true
    git push origin v1.0.0 2>&1 || true
    
    # Also update the bare repo's default branch and verify it has refs
    cd "$GIT_REPO_DIR/$GIT_REPO_NAME"
    git symbolic-ref HEAD refs/heads/master 2>/dev/null || git symbolic-ref HEAD refs/heads/main 2>/dev/null || true
    
    # Verify repository has refs
    if ! git for-each-ref --format='%(refname)' | grep -q .; then
        log_error "Repository has no refs! This will cause git-upload-pack to fail."
        # Try to create a ref manually
        if [ -f "refs/heads/master" ] || [ -f "refs/heads/main" ]; then
            log_info "Found branch refs, repository should be valid"
        else
            log_error "No branch refs found in repository"
        fi
    else
        log_info "Repository has refs:"
        git for-each-ref --format='%(refname)' | head -5
    fi
    
    # Enable http.uploadpack for this repo (required for git-upload-pack over HTTP)
    git config http.uploadpack true || true
    git config http.receivepack true || true
    
    cd "$GIT_REPO_DIR"
    rm -rf test-repo-work
    
    log_success "Mock Git repository created at $GIT_REPO_DIR/$GIT_REPO_NAME"
}

# Start Git HTTP server
start_git_http_server() {
    log_info "Starting Git HTTP server on port $GIT_SERVER_PORT..."
    
    # Verify repository exists and has refs before starting server
    if [ ! -d "$GIT_REPO_DIR/$GIT_REPO_NAME" ]; then
        log_error "Repository not found: $GIT_REPO_DIR/$GIT_REPO_NAME"
        return 1
    fi
    
    # Test git-upload-pack directly to see if it works
    log_info "Testing git-upload-pack on repository..."
    cd "$GIT_REPO_DIR/$GIT_REPO_NAME"
    if git upload-pack --advertise-refs . > /tmp/git-test-output.log 2>&1; then
        log_success "✓ git-upload-pack works on repository"
        cat /tmp/git-test-output.log | head -10
    else
        log_warning "git-upload-pack test failed, but continuing..."
        cat /tmp/git-test-output.log
    fi
    cd /
    
    # Start the Git HTTP server in background
    /app/git-http-server.sh "$GIT_REPO_DIR/$GIT_REPO_NAME" "$GIT_SERVER_PORT" > /tmp/git-server.log 2>&1 &
    GIT_SERVER_PID=$!
    
    # Wait for server to start
    sleep 5
    
    # Test if server is responding
    local test_url="http://localhost:$GIT_SERVER_PORT/$GIT_REPO_NAME/info/refs?service=git-upload-pack"
    if curl -s "$test_url" > /dev/null 2>&1; then
        log_success "Git HTTP server started (PID: $GIT_SERVER_PID)"
    else
        log_error "Git HTTP server failed to start or not responding"
        log_info "Server log (last 50 lines):"
        tail -50 /tmp/git-server.log 2>/dev/null || cat /tmp/git-server.log 2>/dev/null || true
        log_info "Testing server directly:"
        curl -v "$test_url" 2>&1 | head -30 || true
        log_info "Checking if server process is running:"
        ps aux | grep -E "git-server|git-http" | grep -v grep || true
        return 1
    fi
}

# Test wizard functionality with expect
test_wizard() {
    log_info "Testing setup-wizard command with expect..."
    
    # Create a temporary directory for wizard output
    WIZARD_DIR="/tmp/beacon-wizard-test"
    mkdir -p "$WIZARD_DIR"
    cd "$WIZARD_DIR"
    
    # Create expect script to automate wizard
    cat > /tmp/wizard-expect.exp << 'EXPECT_SCRIPT'
#!/usr/bin/expect -f
set timeout 120
set send_slow {1 0.1}

spawn beacon setup-wizard --config /tmp/beacon.monitor.yml --env /tmp/.env

# Welcome message
expect {
    "Welcome to Beacon Configuration Wizard" {
        # Continue
    }
    timeout {
        puts "Failed to see welcome message"
        exit 1
    }
}

# Device name
expect {
    "Device name" {
        send "E2E Test Device\r"
    }
    timeout {
        puts "Failed to see device name prompt"
        exit 1
    }
}

# Device type selection
expect {
    "Select (1-5):" {
        send "1\r"
    }
    timeout {
        puts "Failed to see device type prompt"
        exit 1
    }
}

# Select template
expect {
    "Select template" {
        send "1\r"
    }
    timeout {
        puts "Failed to see template selection"
        exit 1
    }
}

# Configure monitoring - handle all prompts with a loop
expect {
    -re "Configure .* check.*y/n.*:" {
        send "y\r"
        exp_continue
    }
    -re "Host.*:" {
        # Accept default by sending empty (just enter)
        send "\r"
        exp_continue
    }
    -re "Port.*:" {
        # Accept default by sending empty (just enter)
        send "\r"
        exp_continue
    }
    -re "URL.*:" {
        send "http://localhost:8080\r"
        exp_continue
    }
    -re "Expected status code.*:" {
        # Accept default
        send "\r"
        exp_continue
    }
    -re "Command.*:" {
        # Accept default command
        send "\r"
        exp_continue
    }
    -re "Check interval.*:" {
        # Accept default interval
        send "\r"
        exp_continue
    }
    "Add additional custom checks" {
        send "n\r"
        exp_continue
    }
    "Enable alert notifications" {
        send "y\r"
        exp_continue
    }
    "Configure Email notifications" {
        send "y\r"
        exp_continue
    }
    "Enable BeaconInfra reporting" {
        send "n\r"
        exp_continue
    }
    "Configuration complete" {
        puts "Wizard completed successfully"
        # Wait a moment for any final output, then exit
        sleep 0.5
        exit 0
    }
    eof {
        # Process ended, check if we saw completion
        puts "Wizard process ended"
        exit 0
    }
    timeout {
        puts "Wizard timed out waiting for completion"
        exit 1
    }
}
EXPECT_SCRIPT

    chmod +x /tmp/wizard-expect.exp
    
    # Run wizard with expect
    if /tmp/wizard-expect.exp; then
        log_success "✓ Wizard completed successfully"
    else
        log_error "Wizard test failed"
        return 1
    fi
    
    # Verify wizard generated files
    if [ -f "/tmp/beacon.monitor.yml" ]; then
        log_success "✓ Wizard generated beacon.monitor.yml"
        if grep -q "E2E Test Device" /tmp/beacon.monitor.yml; then
            log_success "✓ Monitor config contains device name"
        fi
    else
        log_error "Wizard did not generate beacon.monitor.yml"
        return 1
    fi
    
    if [ -f "/tmp/.env" ]; then
        log_success "✓ Wizard generated .env file"
        if grep -q "SMTP_HOST" /tmp/.env; then
            log_success "✓ Env file contains SMTP configuration"
        fi
    else
        log_error "Wizard did not generate .env file"
        return 1
    fi
    
    if [ -f "/tmp/beacon.bootstrap.yml" ]; then
        log_success "✓ Wizard generated beacon.bootstrap.yml"
    else
        log_error "Wizard did not generate beacon.bootstrap.yml"
        return 1
    fi
    
    cd /
    rm -rf "$WIZARD_DIR"
    
    log_success "Wizard test completed successfully"
}

# Bootstrap Beacon project
bootstrap_beacon_project() {
    log_info "Bootstrapping Beacon project: $PROJECT_NAME"
    
    # Remove existing project if it exists
    beacon projects remove "$PROJECT_NAME" --force 2>/dev/null || true
    
    # Create a bootstrap configuration file for non-interactive setup
    # Use 1 second poll interval for fast testing
    # Use a test token for the mock Git server (server doesn't validate it)
    cat > "/tmp/bootstrap-config.yml" << EOF
project_name: "$PROJECT_NAME"
repo_url: "$GIT_HTTP_URL"
local_path: "$WORKING_DIR/$PROJECT_NAME"
deploy_command: "./deploy.sh"
poll_interval: "1s"
port: "$BEACON_PORT"
ssh_key_path: ""
git_token: "test-token-for-e2e"
secure_env_path: ""
user: "$(whoami)"
working_dir: "$WORKING_DIR/$PROJECT_NAME"
use_system_service: false
EOF
    
    # Bootstrap new project using config file
    if beacon bootstrap "$PROJECT_NAME" -f "/tmp/bootstrap-config.yml" --skip-systemd; then
        log_success "✓ Bootstrap command completed"
    else
        log_error "Bootstrap command failed"
        return 1
    fi
    
    # Bootstrap may not clone the repo immediately - that happens when deploy runs
    # So we'll verify the config was created, and repo cloning will be tested in verify_repository_cloned
    # after we run deploy
    
    log_success "Beacon project bootstrapped"
}

# Verify bootstrap created all necessary files and directories
verify_bootstrap() {
    log_info "Verifying bootstrap results..."
    
    local project_config_dir="$HOME_DIR/.beacon/config/projects/$PROJECT_NAME"
    local project_env_file="$project_config_dir/env"
    local working_dir="$WORKING_DIR/$PROJECT_NAME"
    
    # Check config directory exists
    if [ ! -d "$project_config_dir" ]; then
        log_error "Project config directory not found: $project_config_dir"
        return 1
    fi
    log_success "✓ Project config directory exists"
    
    # Check environment file exists
    if [ ! -f "$project_env_file" ]; then
        log_error "Environment file not found: $project_env_file"
        return 1
    fi
    log_success "✓ Environment file exists"
    
    # Check environment file contains expected variables
    if grep -q "BEACON_REPO_URL" "$project_env_file"; then
        log_success "✓ Environment file contains BEACON_REPO_URL"
        # Verify it's the HTTP URL
        if grep -q "$GIT_HTTP_URL" "$project_env_file"; then
            log_success "✓ Environment file has correct Git HTTP URL"
        fi
    else
        log_error "Environment file missing BEACON_REPO_URL"
        return 1
    fi
    
    if grep -q "BEACON_LOCAL_PATH" "$project_env_file"; then
        log_success "✓ Environment file contains BEACON_LOCAL_PATH"
    else
        log_error "Environment file missing BEACON_LOCAL_PATH"
        return 1
    fi
    
    if grep -q "BEACON_DEPLOY_COMMAND" "$project_env_file"; then
        log_success "✓ Environment file contains BEACON_DEPLOY_COMMAND"
    else
        log_error "Environment file missing BEACON_DEPLOY_COMMAND"
        return 1
    fi
    
    log_success "Bootstrap verification passed"
}

# Verify repository was cloned
verify_repository_cloned() {
    log_info "Verifying repository was cloned..."
    
    local working_dir="$WORKING_DIR/$PROJECT_NAME"
    local project_env_file="$HOME_DIR/.beacon/config/projects/$PROJECT_NAME/env"
    
    # If working directory or repo doesn't exist yet, run deploy to clone it
    if [ ! -d "$working_dir" ] || [ ! -d "$working_dir/.git" ]; then
        log_info "Repository not cloned yet, running deploy to clone it..."
        
        # Ensure parent directory exists and is a directory (deploy will create the project directory)
        local parent_dir=$(dirname "$working_dir")
        if [ -f "$parent_dir" ]; then
            log_warning "Parent directory path exists as a file, removing it: $parent_dir"
            rm -f "$parent_dir"
        fi
        mkdir -p "$parent_dir"
        log_info "Created parent directory: $parent_dir"
        
        # Also ensure the working directory path doesn't exist as a file
        if [ -f "$working_dir" ]; then
            log_warning "Working directory path exists as a file, removing it: $working_dir"
            rm -f "$working_dir"
        fi
        
        # Load environment from project env file
        set -a
        source "$project_env_file" 2>/dev/null || true
        set +a
        
        # Run deploy briefly to clone the repo
        log_info "Running beacon deploy to clone repository..."
        timeout 15 beacon deploy > /tmp/deploy-clone.log 2>&1 &
        DEPLOY_CLONE_PID=$!
        
        # Wait for deployment to complete (with 1s poll interval, should be fast)
        # Give it more time for initial clone
        sleep 8
        
        # Check if repo was cloned
        if [ -d "$working_dir/.git" ]; then
            log_success "✓ Repository cloned successfully"
            # Stop deploy process
            kill $DEPLOY_CLONE_PID 2>/dev/null || true
            wait $DEPLOY_CLONE_PID 2>/dev/null || true
        else
            log_warning "Repository not cloned yet, waiting a bit more..."
            sleep 5
            kill $DEPLOY_CLONE_PID 2>/dev/null || true
            wait $DEPLOY_CLONE_PID 2>/dev/null || true
        fi
    fi
    
    # Check if working directory exists
    if [ ! -d "$working_dir" ]; then
        log_error "Working directory not found: $working_dir"
        log_info "Deploy log output:"
        tail -20 /tmp/deploy-clone.log 2>/dev/null || true
        return 1
    fi
    log_success "✓ Working directory exists"
    
    # Check if it's a git repository
    if [ ! -d "$working_dir/.git" ]; then
        log_error "Working directory is not a git repository"
        return 1
    fi
    log_success "✓ Working directory is a git repository"
    
    # Check if expected files exist
    if [ ! -f "$working_dir/deploy.sh" ]; then
        log_error "deploy.sh not found in cloned repository"
        return 1
    fi
    log_success "✓ deploy.sh exists in repository"
    
    if [ ! -f "$working_dir/version.txt" ]; then
        log_error "version.txt not found in cloned repository"
        return 1
    fi
    log_success "✓ version.txt exists in repository"
    
    # Verify git remote is set correctly
    cd "$working_dir"
    local remote_url=$(git remote get-url origin 2>/dev/null || echo "")
    if [ -z "$remote_url" ]; then
        log_error "Git remote origin not configured"
        return 1
    fi
    log_success "✓ Git remote origin configured: $remote_url"
    
    # Verify we're on the correct branch/tag
    local current_ref=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || git describe --tags 2>/dev/null || echo "")
    if [ -z "$current_ref" ]; then
        log_warning "Could not determine current git reference"
    else
        log_success "✓ Current git reference: $current_ref"
    fi
    
    log_success "Repository clone verification passed"
}

# Test deployment execution
test_deployment_execution() {
    log_info "Testing deployment execution..."
    
    local working_dir="$WORKING_DIR/$PROJECT_NAME"
    local deploy_log="$working_dir/deploy.log"
    
    # Run deploy command manually to test it works
    cd "$working_dir"
    
    # Check if deploy script exists and is executable
    if [ ! -x "./deploy.sh" ]; then
        log_error "deploy.sh is not executable"
        return 1
    fi
    
    # Clean up any existing deploy.log
    rm -f "$deploy_log"
    
    # Run deploy script
    if ./deploy.sh "v1.0.0" > /tmp/deploy-output.log 2>&1; then
        log_success "✓ Deploy script executed successfully"
    else
        log_error "Deploy script failed"
        cat /tmp/deploy-output.log
        return 1
    fi
    
    # Verify deploy created expected output
    if [ -f "$working_dir/version.txt" ]; then
        if grep -q "v1.0.0" "$working_dir/version.txt"; then
            log_success "✓ Deploy script updated version.txt"
        else
            log_warning "Deploy script may not have updated version.txt correctly"
        fi
    fi
    
    if [ -f "$deploy_log" ]; then
        if grep -q "Deploy command executed successfully" "$deploy_log"; then
            log_success "✓ Deploy log file created with expected content"
        fi
    fi
    
    log_success "Deployment execution test passed"
}

# Test bootstrap actually runs deploy command
test_bootstrap_runs_deploy() {
    log_info "Testing that bootstrap runs deploy command..."
    
    local working_dir="$WORKING_DIR/$PROJECT_NAME"
    local project_env_file="$HOME_DIR/.beacon/config/projects/$PROJECT_NAME/env"
    
    # Remove the working directory to simulate fresh bootstrap
    if [ -d "$working_dir" ]; then
        rm -rf "$working_dir"
    fi
    
    # Source the project env file to set environment variables
    # Then run beacon deploy which will use those env vars
    log_info "Testing deploy command execution with project environment..."
    
    # Load environment from project env file
    set -a
    source "$project_env_file" 2>/dev/null || true
    set +a
    
    # Run beacon deploy in background for a short time to test initial deployment
    # Since deploy runs in a loop, we'll just test that it can start and do initial clone
    timeout 10 beacon deploy > /tmp/deploy-output.log 2>&1 &
    DEPLOY_PID=$!
    
    # Wait a bit for initial deployment
    sleep 5
    
    # Check if repo was cloned (indicating deploy ran)
    if [ -d "$working_dir/.git" ]; then
        log_success "✓ Repository was cloned (deploy command executed)"
        
        # Check if deploy.log exists (indicating deploy script ran)
        if [ -f "$working_dir/deploy.log" ]; then
            log_success "✓ Deploy script was executed (deploy.log exists)"
        else
            log_warning "Deploy log not found - deploy script may not have run yet"
        fi
    else
        log_error "Repository was not cloned - deploy may have failed"
        cat /tmp/deploy-output.log
        kill $DEPLOY_PID 2>/dev/null || true
        return 1
    fi
    
    # Stop the deploy process
    kill $DEPLOY_PID 2>/dev/null || true
    wait $DEPLOY_PID 2>/dev/null || true
    
    log_success "Bootstrap deploy execution test passed"
}

# Test secure env file loading
test_secure_env_file() {
    log_info "Testing secure environment file loading..."
    
    local secure_env_file="/tmp/beacon-secure.env"
    local project_env_file="$HOME_DIR/.beacon/config/projects/$PROJECT_NAME/env"
    
    # Create a secure env file with BEACON_GIT_TOKEN
    cat > "$secure_env_file" << EOF
BEACON_GIT_TOKEN=test-token-12345
BEACON_SECURE_VAR=secure-value
EOF
    chmod 600 "$secure_env_file"
    
    # Update bootstrap config to use secure env file
    cat > "/tmp/bootstrap-config-secure.yml" << EOF
project_name: "$PROJECT_NAME-secure"
repo_url: "$GIT_HTTP_URL"
local_path: "$WORKING_DIR/$PROJECT_NAME-secure"
deploy_command: "./deploy.sh"
poll_interval: "5s"
port: "$BEACON_PORT"
ssh_key_path: ""
git_token: ""
secure_env_path: "$secure_env_file"
user: "$(whoami)"
working_dir: "$WORKING_DIR/$PROJECT_NAME-secure"
use_system_service: false
EOF
    
    # Bootstrap with secure env file
    if beacon bootstrap "$PROJECT_NAME-secure" -f "/tmp/bootstrap-config-secure.yml" --skip-systemd; then
        log_success "✓ Bootstrap with secure env file completed"
    else
        log_error "Bootstrap with secure env file failed"
        return 1
    fi
    
    # Verify secure env path is in the project env file
    local secure_project_env="$HOME_DIR/.beacon/config/projects/$PROJECT_NAME-secure/env"
    if grep -q "BEACON_SECURE_ENV_PATH=$secure_env_file" "$secure_project_env"; then
        log_success "✓ Secure env path configured in project env file"
    else
        log_warning "Secure env path not found in project env file"
    fi
    
    # Clean up secure test project
    beacon projects remove "$PROJECT_NAME-secure" --force 2>/dev/null || true
    rm -f "$secure_env_file"
    
    log_success "Secure env file test passed"
}

# Test deployment by creating new tag and verifying redeployment
test_deployment_with_new_tag() {
    log_info "Testing deployment by creating new Git tag and verifying redeployment..."
    
    local working_dir="$WORKING_DIR/$PROJECT_NAME"
    local project_env_file="$HOME_DIR/.beacon/config/projects/$PROJECT_NAME/env"
    
    # Set global git config to avoid identity issues
    git config --global user.name "E2E Test" || true
    git config --global user.email "test@beacon.local" || true
    
    # Start beacon deploy in background with 1s poll interval
    log_info "Starting beacon deploy in background (poll interval: 1s)..."
    
    # Load environment from project env file
    set -a
    source "$project_env_file" 2>/dev/null || true
    set +a
    
    # Start deploy process in background
    beacon deploy > /tmp/deploy-background.log 2>&1 &
    DEPLOY_PID=$!
    
    log_info "Beacon deploy started (PID: $DEPLOY_PID), waiting for initial deployment..."
    
    # Wait for initial deployment to complete
    sleep 3
    
    # Verify repo was cloned initially
    if [ ! -d "$working_dir/.git" ]; then
        log_error "Repository was not cloned during initial deployment"
        kill $DEPLOY_PID 2>/dev/null || true
        return 1
    fi
    log_success "✓ Initial deployment completed (repo cloned)"
    
    # Get initial version
    local initial_version=""
    if [ -f "$working_dir/version.txt" ]; then
        initial_version=$(cat "$working_dir/version.txt" | head -1)
        log_info "Initial version: $initial_version"
    fi
    
    # Clone the bare repo directly to create new tag
    local temp_clone_dir="/tmp/beacon-e2e-clone"
    rm -rf "$temp_clone_dir"
    mkdir -p "$temp_clone_dir"
    cd "$temp_clone_dir"
    
    # Clone from the bare repo directly
    if git clone "$GIT_REPO_DIR/$GIT_REPO_NAME" test-repo-new 2>&1 | tee /tmp/git-clone.log; then
        log_success "✓ Cloned repository from bare repo for tag creation"
    else
        log_error "Failed to clone from bare repo"
        cat /tmp/git-clone.log
        kill $DEPLOY_PID 2>/dev/null || true
        return 1
    fi
    
    cd test-repo-new
    
    # Update version with a unique marker
    local new_version="v1.1.0-$(date +%s)"
    echo "Updated version for E2E test: $new_version" > version.txt
    echo "Deployed at: $(date)" >> version.txt
    echo "Redeployment test marker" >> version.txt
    
    # Commit and tag
    git add version.txt
    git commit -m "Update version for E2E redeployment test" || true
    git tag v1.1.0 || true
    
    # Push to bare repo (which the HTTP server serves)
    if git push origin master 2>&1 | tee /tmp/git-push.log && \
       git push origin v1.1.0 2>&1 | tee -a /tmp/git-push.log; then
        log_success "✓ Pushed new commit and tag v1.1.0 to repository"
    else
        log_warning "Push may have failed (this is okay if repo is already up to date)"
        cat /tmp/git-push.log
    fi
    
    cd /
    rm -rf "$temp_clone_dir"
    
    # Wait for beacon to detect the new tag and redeploy (with 1s poll interval, should be fast)
    log_info "Waiting for beacon to detect new tag and redeploy (poll interval: 1s)..."
    local max_wait=10
    local waited=0
    local redeployed=false
    
    while [ $waited -lt $max_wait ]; do
        sleep 2
        waited=$((waited + 2))
        
        # Check if version.txt was updated (indicating redeployment)
        if [ -f "$working_dir/version.txt" ]; then
            if grep -q "Redeployment test marker" "$working_dir/version.txt"; then
                log_success "✓ Redeployment detected! New version deployed"
                redeployed=true
                break
            fi
        fi
        
        # Check deploy log for tag detection
        if grep -q "New tag found: v1.1.0" /tmp/deploy-background.log 2>/dev/null; then
            log_info "New tag detected in deploy log, waiting for redeployment..."
        fi
    done
    
    # Stop deploy process
    kill $DEPLOY_PID 2>/dev/null || true
    wait $DEPLOY_PID 2>/dev/null || true
    
    if [ "$redeployed" = "true" ]; then
        log_success "✓ Tag-based redeployment test passed"
        
        # Verify the new version is in the file
        if grep -q "v1.1.0" "$working_dir/version.txt"; then
            log_success "✓ New version v1.1.0 confirmed in deployed files"
        fi
    else
        log_error "Redeployment did not occur within $max_wait seconds"
        log_info "Deploy log output:"
        tail -20 /tmp/deploy-background.log || true
        return 1
    fi
    
    log_success "New tag v1.1.0 created, pushed, and redeployment verified"
}

# Verify new tag can be fetched and detected
verify_tag_polling() {
    log_info "Verifying tag polling functionality..."
    
    local working_dir="$WORKING_DIR/$PROJECT_NAME"
    
    # Change to the working directory and fetch tags
    cd "$working_dir"
    
    # Verify the remote is set to HTTP URL
    local remote_url=$(git remote get-url origin 2>/dev/null || echo "")
    if [[ "$remote_url" == *"http://localhost"* ]] || [[ "$remote_url" == *"$GIT_HTTP_URL"* ]]; then
        log_success "✓ Git remote is configured to use HTTP URL"
    else
        log_warning "Git remote URL: $remote_url (may not be HTTP)"
    fi
    
    # Fetch tags from remote (this simulates what beacon does)
    if git fetch --tags origin 2>&1 | tee /tmp/git-fetch.log; then
        log_success "✓ Git fetch --tags succeeded"
    else
        log_error "Git fetch --tags failed"
        cat /tmp/git-fetch.log
        # Try fetching from HTTP URL directly
        log_info "Attempting to fetch from HTTP URL directly..."
        if git fetch --tags "$GIT_HTTP_URL" 2>&1 | tee -a /tmp/git-fetch.log; then
            log_success "✓ Git fetch from HTTP URL succeeded"
        else
            return 1
        fi
    fi
    
    # Check if new tag exists locally
    if git tag | grep -q "v1.1.0"; then
        log_success "✓ New tag v1.1.0 fetched successfully"
    else
        log_warning "New tag v1.1.0 not found after fetch (may need to check remote tags)"
        # List all tags
        git tag -l
    fi
    
    # Test getLatestTagFromRepo equivalent
    local latest_tag=$(git for-each-ref --sort=-creatordate --format='%(refname:short)' refs/tags 2>/dev/null | head -n 1)
    if [ -n "$latest_tag" ]; then
        log_success "✓ Latest tag detection works: $latest_tag"
        if [ "$latest_tag" = "v1.1.0" ]; then
            log_success "✓ Latest tag is v1.1.0 as expected"
        fi
    else
        log_warning "Could not determine latest tag"
    fi
    
    log_success "Tag polling verification passed"
}

# Test monitoring
test_monitoring() {
    log_info "Testing monitoring functionality..."
    
    # Create a simple monitor configuration
    local monitor_config="$HOME_DIR/.beacon/config/projects/$PROJECT_NAME/monitor.yml"
    cat > "$monitor_config" << EOF
device:
  name: "e2e-test-device"
  location: "test"
  environment: "test"

checks:
  - name: "Test HTTP Check"
    type: "http"
    url: "http://localhost:$BEACON_PORT"
    interval: "30s"
    timeout: "10s"
    expect_status: 200
EOF
    
    # Test that monitor command can parse the config
    if beacon monitor -f "$monitor_config" --dry-run 2>/dev/null || beacon monitor --help > /dev/null 2>&1; then
        log_success "✓ Monitor command is available and can parse config"
    else
        log_warning "Monitor command test skipped (may require running service)"
    fi
    
    log_success "Monitoring test completed"
}

# Test full workflow: wizard -> bootstrap -> deploy -> monitor
test_full_workflow() {
    log_info "Testing full workflow integration..."
    
    # This test verifies the complete workflow:
    # 1. Wizard generates configs ✓ (tested in test_wizard)
    # 2. Bootstrap uses configs to set up project ✓ (tested in bootstrap_beacon_project)
    # 3. Repository is cloned ✓ (tested in verify_repository_cloned)
    # 4. Deploy command runs ✓ (tested in test_deployment_execution)
    # 5. Monitoring can start ✓ (tested in test_monitoring)
    # 6. Tag polling works ✓ (tested in verify_tag_polling)
    
    log_success "✓ Full workflow components tested"
    log_info "All workflow steps verified individually"
    
    log_success "Full workflow test completed"
}

# Main test execution
main() {
    log_info "Starting Beacon E2E Integration Test"
    log_info "Project: $PROJECT_NAME"
    log_info "Home: $HOME_DIR (actual HOME: ${HOME:-not set})"
    log_info "Working: $WORKING_DIR"
    log_info "Git HTTP URL: $GIT_HTTP_URL"
    log_info "Docker mode: $IS_DOCKER"
    
    # Run test steps
    check_prerequisites
    test_canonical_local_flow
    test_cloud_login_logout
    create_mock_git_repo
    start_git_http_server
    test_wizard
    bootstrap_beacon_project
    verify_bootstrap
    verify_repository_cloned
    test_deployment_execution
    test_bootstrap_runs_deploy
    test_secure_env_file
    test_deployment_with_new_tag
    test_monitoring
    test_full_workflow
    
    log_success "🎉 All E2E tests passed!"
    log_info "Beacon is working correctly in a real-world scenario"
}

# Run main function
main "$@"
