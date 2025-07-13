#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Beacon repository details
GITHUB_API="https://api.github.com/repos/Bajusz15/beacon"

echo -e "${BLUE}=== Beacon Agent Installer ===${NC}"

# Function to detect system architecture
detect_architecture() {
    local arch
    local os
    
    # Detect OS
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        os="linux"
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        os="darwin"
    else
        echo -e "${RED}Unsupported OS: $OSTYPE${NC}"
        exit 1
    fi
    
    # Detect architecture
    case "$(uname -m)" in
        x86_64)
            arch="amd64"
            ;;
        aarch64|arm64)
            arch="arm64"
            ;;
        armv7l|armv6l)
            arch="arm"
            ;;
        *)
            echo -e "${RED}Unsupported architecture: $(uname -m)${NC}"
            exit 1
            ;;
    esac
    
    echo "${os}_${arch}"
}

# Function to get latest release version
get_latest_version() {
    local version
    version=$(curl -s "$GITHUB_API/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [[ -z "$version" ]]; then
        echo -e "${RED}Failed to get latest version from GitHub${NC}"
        exit 1
    fi
    
    echo "$version"
}

# Function to download binary
download_binary() {
    local version="$1"
    local arch="$2"
    local download_url="https://github.com/Bajusz15/beacon/releases/download/$version/beacon-$arch"
    
    echo -e "${YELLOW}Downloading Beacon $version for $arch...${NC}"
    
    # Create temporary directory
    local temp_dir=$(mktemp -d)
    cd "$temp_dir"
    
    # Download binary
    if ! curl -L -o beacon "$download_url"; then
        echo -e "${RED}Failed to download binary from $download_url${NC}"
        exit 1
    fi
    
    # Make executable
    chmod +x beacon
    
    # Copy to system
    echo -e "${YELLOW}Installing to /usr/local/bin/beacon...${NC}"
    sudo cp beacon /usr/local/bin/beacon
    
    # Cleanup
    cd - > /dev/null
    rm -rf "$temp_dir"
    
    echo -e "${GREEN}Binary installed successfully!${NC}"
}

# Check if running as root
if [[ $EUID -eq 0 ]]; then
    echo -e "${RED}Please do not run this script as root${NC}"
    exit 1
fi

# Check dependencies
if ! command -v curl &> /dev/null; then
    echo -e "${RED}curl is required but not installed. Please install curl first.${NC}"
    exit 1
fi

# Detect architecture
echo -e "${BLUE}Detecting system architecture...${NC}"
ARCH=$(detect_architecture)
echo -e "${GREEN}Detected: $ARCH${NC}"

# Get latest version
echo -e "${BLUE}Getting latest version...${NC}"
VERSION=$(get_latest_version)
echo -e "${GREEN}Latest version: $VERSION${NC}"

# Check if binary already exists
if [[ -f /usr/local/bin/beacon ]]; then
    echo -e "${YELLOW}Beacon binary already exists at /usr/local/bin/beacon${NC}"
    read -p "Do you want to overwrite it? (y/N): " REPLY
    if [[ ! "$REPLY" =~ ^[Yy]$ ]]; then
        echo -e "${YELLOW}Installation cancelled${NC}"
        exit 0
    fi
    sudo rm -f /usr/local/bin/beacon
fi

# Download and install binary
download_binary "$VERSION" "$ARCH"

# Create systemd service template if it doesn't exist
if [[ ! -f /etc/systemd/system/beacon@.service ]]; then
    echo -e "${YELLOW}Installing systemd service template...${NC}"
    
    # Create the service file
    sudo tee /etc/systemd/system/beacon@.service > /dev/null <<EOF
[Unit]
Description=Beacon Agent for %i - Lightweight deployment and reporting for IoT
After=network.target

[Service]
EnvironmentFile=/etc/beacon/projects/%i/env
Type=simple
ExecStart=/usr/local/bin/beacon
WorkingDirectory=$HOME/beacon/%i
Restart=always
RestartSec=5
User=pi

# Logging
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    echo -e "${GREEN}Systemd service template installed${NC}"
fi

# Create directories
echo -e "${YELLOW}Creating directories...${NC}"
sudo mkdir -p /etc/beacon/projects
sudo mkdir -p $HOME/beacon

echo -e "${GREEN}=== Beacon installation complete! ===${NC}"
echo
echo -e "${BLUE}Next steps:${NC}"
echo "1. Create a project configuration:"
echo "   sudo mkdir -p /etc/beacon/projects/myapp"
echo "   sudo cp beacon.env.example /etc/beacon/projects/myapp/env"
echo "   sudo nano /etc/beacon/projects/myapp/env"
echo
echo "2. Create deployment directory:"
echo "   sudo mkdir -p $HOME/beacon/myapp"
echo
echo "3. Start the service:"
echo "   sudo systemctl enable --now beacon@myapp"
echo
echo -e "${BLUE}For more information, see the README.md file.${NC}"
