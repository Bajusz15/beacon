#!/bin/bash

set -e

echo "=== Beacon Agent Installer ==="

# Prompt for project name
read -rp "Enter project name (e.g. myapp): " PROJECT
if [[ -z "$PROJECT" ]]; then
  echo "Project name cannot be empty."
  exit 1
fi

# Prompt for repo URL
read -rp "Enter Git repository URL: " REPO_URL

# Prompt for SSH key path (optional)
read -rp "Enter SSH private key path (leave blank if not using SSH): " SSH_KEY_PATH

# Prompt for Git token (optional)
read -rp "Enter Git personal access token (leave blank if not using HTTPS): " GIT_TOKEN

# Prompt for deployment path
read -rp "Enter local deployment path [/opt/beacon/$PROJECT]: " LOCAL_PATH
LOCAL_PATH=${LOCAL_PATH:-/opt/beacon/$PROJECT}

# Prompt for poll interval
read -rp "Enter poll interval [60s]: " POLL_INTERVAL
POLL_INTERVAL=${POLL_INTERVAL:-60s}

# Prompt for HTTP port
read -rp "Enter HTTP server port [8080]: " PORT
PORT=${PORT:-8080}

# Create env file directory
sudo mkdir -p /etc/beacon/projects/$PROJECT
sudo mkdir -p "$LOCAL_PATH"

# Write env file
sudo tee /etc/beacon/projects/$PROJECT/env > /dev/null <<EOF
BEACON_REPO_URL=$REPO_URL
BEACON_SSH_KEY_PATH=$SSH_KEY_PATH
BEACON_GIT_TOKEN=$GIT_TOKEN
BEACON_LOCAL_PATH=$LOCAL_PATH
BEACON_POLL_INTERVAL=$POLL_INTERVAL
BEACON_PORT=$PORT
EOF

# Copy binary
if [[ ! -f /usr/local/bin/beacon ]]; then
  echo "Copying beacon binary to /usr/local/bin..."
  sudo cp ./beacon /usr/local/bin/beacon
  sudo chmod +x /usr/local/bin/beacon
fi

# Copy systemd template if not present
if [[ ! -f /etc/systemd/system/beacon@.service ]]; then
  echo "Copying systemd template..."
  sudo cp ./systemd/beacon@.service /etc/systemd/system/beacon@.service
fi

# Reload systemd
sudo systemctl daemon-reload

# Enable and start the service
sudo systemctl enable --now beacon@"$PROJECT"

echo "=== Beacon Agent installed and started for project: $PROJECT ==="
echo "Edit /etc/beacon/projects/$PROJECT/env to change configuration."
