#!/usr/bin/env bash

# Beacon — Proxmox VE host installer (overseer role)
# https://beaconinfra.dev  •  https://github.com/Bajusz15/beacon
# License: MIT
#
# Installs the Beacon agent directly on a Proxmox VE host so it can act as an
# "overseer": monitor the host and see every VM/container it runs (including the
# up/down state a crashed guest can't report for itself) via the host's own pvesh.
#
# Run on the Proxmox host as root:
#   bash -c "$(curl -fsSL https://raw.githubusercontent.com/Bajusz15/beacon/main/scripts/proxmox-install.sh)"
#
# Non-interactive:
#   BEACON_API_KEY=usr_xxx bash -c "$(curl -fsSL .../proxmox-install.sh)"

set -Eeuo pipefail

# ---------- pretty output ----------------------------------------------------
RD=$'\033[01;31m'; GN=$'\033[1;92m'; YW=$'\033[33m'; BL=$'\033[1;34m'; CL=$'\033[m'
CM="${GN}✓${CL}"; CROSS="${RD}✗${CL}"; INFO="${BL}•${CL}"
msg_info()  { echo -e " ${INFO} ${YW}$1...${CL}"; }
msg_ok()    { echo -e " ${CM} ${GN}$1${CL}"; }
msg_err()   { echo -e " ${CROSS} ${RD}$1${CL}"; }

on_error() { msg_err "Installation failed on line $1."; exit 1; }
trap 'on_error $LINENO' ERR

GITHUB_REPO="Bajusz15/beacon"
BIN_PATH="/usr/local/bin/beacon"
SERVICE_PATH="/etc/systemd/system/beacon.service"

header() {
  clear 2>/dev/null || true
  cat <<'EOF'
   ___
  / _ )___ ___ ____ ___  ___
 / _  / -_) _ `/ __/ _ \/ _ \
/____/\__/\_,_/\__/\___/_//_/   Proxmox overseer installer
EOF
  echo
}

# ---------- preflight --------------------------------------------------------
require_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    msg_err "Run this on the Proxmox host as root."
    exit 1
  fi
}

require_pve() {
  if ! command -v pveversion >/dev/null 2>&1 || ! command -v pvesh >/dev/null 2>&1; then
    msg_err "This does not look like a Proxmox VE host (pveversion/pvesh not found)."
    msg_err "Install Beacon inside a guest with the standard installer instead:"
    echo   "    curl -fsSL https://get.beaconinfra.dev | bash"
    exit 1
  fi
  msg_ok "Proxmox VE detected: $(pveversion | head -n1)"
}

require_deps() {
  for dep in curl systemctl; do
    command -v "$dep" >/dev/null 2>&1 || { msg_err "$dep is required but not installed."; exit 1; }
  done
}

detect_arch() {
  case "$(uname -m)" in
    x86_64)        echo "linux_amd64" ;;
    aarch64|arm64) echo "linux_arm64" ;;
    *) msg_err "Unsupported architecture: $(uname -m)"; exit 1 ;;
  esac
}

# ---------- install ----------------------------------------------------------
install_binary() {
  local arch version url tmp
  arch="$(detect_arch)"

  msg_info "Resolving latest Beacon release"
  version="$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
    | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')"
  [[ -n "$version" ]] || { msg_err "Could not determine latest version."; exit 1; }
  msg_ok "Latest release: ${version} (${arch})"

  url="https://github.com/${GITHUB_REPO}/releases/download/${version}/beacon-${arch}"
  tmp="$(mktemp)"
  msg_info "Downloading ${url}"
  curl -fsSL -o "$tmp" "$url"
  install -m 0755 "$tmp" "$BIN_PATH"
  rm -f "$tmp"
  msg_ok "Installed $("$BIN_PATH" version 2>/dev/null || echo beacon) to ${BIN_PATH}"
}

prompt_api_key() {
  # Precedence: env var → interactive prompt. Never echoed to the terminal.
  if [[ -n "${BEACON_API_KEY:-}" ]]; then
    msg_ok "Using API key from BEACON_API_KEY"
    return
  fi
  if [[ ! -t 0 ]]; then
    msg_err "No API key. Set BEACON_API_KEY=usr_... or run interactively."
    msg_err "Get one at https://beaconinfra.dev (Settings → API Keys)."
    exit 1
  fi
  echo
  echo -e " ${INFO} Get an API key at ${BL}https://beaconinfra.dev${CL} (Settings → API Keys)."
  read -rsp "   Paste your Beacon API key (usr_...): " BEACON_API_KEY
  echo
  [[ -n "$BEACON_API_KEY" ]] || { msg_err "No API key entered."; exit 1; }
}

cloud_login() {
  local name="${BEACON_DEVICE_NAME:-$(hostname)}"
  msg_info "Registering this host with BeaconInfra as '${name}'"
  "$BIN_PATH" cloud login --api-key "$BEACON_API_KEY" --name "$name" >/dev/null
  msg_ok "Cloud credentials saved (~/.beacon/config.yaml)"
}

install_service() {
  msg_info "Installing systemd service"
  cat >"$SERVICE_PATH" <<EOF
[Unit]
Description=Beacon agent (Proxmox overseer)
Documentation=https://beaconinfra.dev
After=network-online.target pve-cluster.service
Wants=network-online.target

[Service]
Type=simple
User=root
Environment=HOME=/root
ExecStart=${BIN_PATH} master --foreground
Restart=always
RestartSec=5
NoNewPrivileges=false

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now beacon.service >/dev/null 2>&1
  msg_ok "beacon.service enabled and started"
}

verify_overseer() {
  msg_info "Verifying overseer access to guests"
  if "$BIN_PATH" overseer status 2>/dev/null; then
    msg_ok "Overseer can see this host's guests"
  else
    msg_err "Overseer could not query pvesh (the agent is still installed and running)."
  fi
}

# ---------- main -------------------------------------------------------------
header
require_root
require_deps
require_pve
install_binary
prompt_api_key
cloud_login
install_service
verify_overseer

echo
msg_ok "Beacon is installed and overseeing this Proxmox host."
echo
echo -e " ${BL}Next steps:${CL}"
echo "   • This host now appears in your dashboard at https://beaconinfra.dev"
echo "   • List its guests:        beacon overseer list"
echo "   • Service status / logs:  systemctl status beacon  •  journalctl -u beacon -f"
echo "   • Install Beacon inside the VMs you want to reach into (terminal/tunnel):"
echo "       curl -fsSL https://get.beaconinfra.dev | bash"
echo "     then assign each VM under this host in the dashboard's Devices view."
echo
