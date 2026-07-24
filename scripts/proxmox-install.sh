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
# Non-interactive (skips the dialogs):
#   BEACON_API_KEY=bci_... bash -c "$(curl -fsSL .../proxmox-install.sh)"
#   Optional: BEACON_DEVICE_NAME="rack-pve-1"

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

# Interactivity: a TTY on both ends enables spinners; whiptail additionally enables
# the dialog UI. Anything piped / non-interactive falls back to env vars + plain prompts.
INTERACTIVE=false; [[ -t 0 && -t 1 ]] && INTERACTIVE=true
USE_TUI=false; $INTERACTIVE && command -v whiptail >/dev/null 2>&1 && USE_TUI=true
USE_SPINNER=$INTERACTIVE

DEVICE_NAME="${BEACON_DEVICE_NAME:-$(hostname)}"
API_KEY="${BEACON_API_KEY:-}"

# spin "message" cmd args...  — run a command with a spinner (falls back to a static
# line when there's no TTY). On failure it prints the captured output and exits.
spin() {
  local msg=$1; shift
  if ! $USE_SPINNER; then
    msg_info "$msg"; "$@"; return
  fi
  local log; log="$(mktemp)"
  "$@" >"$log" 2>&1 &
  local pid=$! i=0 fr='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
  while kill -0 "$pid" 2>/dev/null; do
    printf "\r ${BL}%s${CL} ${YW}%s...${CL}" "${fr:i++%${#fr}:1}" "$msg"
    sleep 0.1
  done
  if wait "$pid"; then
    printf "\r ${CM} ${GN}%s${CL}\n" "$msg"; rm -f "$log"
  else
    printf "\r ${CROSS} ${RD}%s${CL}\n" "$msg"; echo; cat "$log"; rm -f "$log"; exit 1
  fi
}

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

require_deps() {
  for dep in curl systemctl; do
    command -v "$dep" >/dev/null 2>&1 || { msg_err "$dep is required but not installed."; exit 1; }
  done
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

# ---------- configuration (Default / Advanced) -------------------------------
configure() {
  if $USE_TUI; then
    if whiptail --title "Beacon Proxmox Overseer" \
         --yes-button "Default" --no-button "Advanced" \
         --yesno "This installs the Beacon agent on this Proxmox host (${DEVICE_NAME}) and registers it with BeaconInfra, so you can see and power-control its VMs and containers from the dashboard.\n\nContinue with default settings, or choose Advanced to change the device name?" \
         15 74; then
      : # Default: keep DEVICE_NAME = hostname
    else
      local rc=$?
      [[ $rc -eq 1 ]] || { msg_err "Cancelled."; exit 1; }   # 255 = ESC/abort
      DEVICE_NAME="$(whiptail --title "Advanced settings" \
        --inputbox "Device name (as it appears in the dashboard):" 10 74 "$DEVICE_NAME" \
        3>&1 1>&2 2>&3)" || { msg_err "Cancelled."; exit 1; }
      [[ -n "$DEVICE_NAME" ]] || DEVICE_NAME="$(hostname)"
    fi
    if [[ -z "$API_KEY" ]]; then
      API_KEY="$(whiptail --title "Beacon API key" \
        --passwordbox "Paste your Beacon API key (bci_...).\n\nGet one at https://beaconinfra.dev  →  Settings → API Keys." 12 74 \
        3>&1 1>&2 2>&3)" || { msg_err "Cancelled."; exit 1; }
    fi
  else
    if [[ -z "$API_KEY" ]]; then
      if ! $INTERACTIVE; then
        msg_err "No API key. Set BEACON_API_KEY=bci_... (get one at https://beaconinfra.dev → Settings → API Keys)."
        exit 1
      fi
      echo
      echo -e " ${INFO} Get an API key at ${BL}https://beaconinfra.dev${CL} (Settings → API Keys)."
      read -rsp "   Paste your Beacon API key (bci_...): " API_KEY
      echo
    fi
  fi
  [[ -n "$API_KEY" ]] || { msg_err "No API key provided."; exit 1; }
}

# ---------- install ----------------------------------------------------------
detect_arch() {
  # Echoes the release suffix, or nothing for an unsupported arch (checked by the caller,
  # so the error surfaces in the main shell rather than being swallowed by $(...)).
  case "$(uname -m)" in
    x86_64)        echo "linux_amd64" ;;
    aarch64|arm64) echo "linux_arm64" ;;
    armv7l|armhf)  echo "linux_arm" ;;
  esac
}

install_binary() {
  local arch version url tmp
  arch="$(detect_arch)"
  [[ -n "$arch" ]] || { msg_err "Unsupported architecture: $(uname -m)"; exit 1; }

  msg_info "Resolving latest Beacon release"
  version="$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
    | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')"
  [[ -n "$version" ]] || { msg_err "Could not determine the latest release (is one published yet?)."; exit 1; }
  msg_ok "Latest release: ${version} (${arch})"

  url="https://github.com/${GITHUB_REPO}/releases/download/${version}/beacon-${arch}"
  tmp="$(mktemp)"
  spin "Downloading beacon-${arch}" curl -fsSL -o "$tmp" "$url"
  install -m 0755 "$tmp" "$BIN_PATH"
  rm -f "$tmp"
  msg_ok "Installed $("$BIN_PATH" version 2>/dev/null || echo beacon) to ${BIN_PATH}"
}

cloud_login() {
  msg_info "Registering this host with BeaconInfra as '${DEVICE_NAME}'"
  "$BIN_PATH" cloud login --api-key "$API_KEY" --name "$DEVICE_NAME" >/dev/null
  msg_ok "Cloud credentials saved (~/.beacon/config.yaml)"
}

install_service() {
  # Remove a service from an older installer (named beacon.service) so we don't end up
  # running two masters; Beacon installs its own canonical beacon-master.service.
  if [[ -f /etc/systemd/system/beacon.service ]]; then
    systemctl disable --now beacon.service >/dev/null 2>&1 || true
    rm -f /etc/systemd/system/beacon.service
  fi
  spin "Installing systemd service (beacon-master)" "$BIN_PATH" service install
}

verify_overseer() {
  msg_info "Verifying overseer access to guests"
  if "$BIN_PATH" overseer status 2>/dev/null; then
    msg_ok "Overseer can see this host's guests"
  else
    msg_err "Overseer could not query pvesh (the agent is still installed and running)."
  fi
}

summary() {
  echo
  msg_ok "Beacon is installed and overseeing this Proxmox host."
  echo
  echo -e " ${BL}Next steps:${CL}"
  echo "   • This host now appears in your dashboard at https://beaconinfra.dev"
  echo "   • List its guests:        beacon proxmox list"
  echo "   • Service status / logs:  systemctl status beacon-master  •  journalctl -u beacon-master -f"
  echo "   • Restart:                beacon restart  •  systemctl restart beacon-master"
  echo "   • (Optional) Install Beacon inside a VM to reach into it (terminal/tunnel):"
  echo "       curl -fsSL https://get.beaconinfra.dev | bash"
  echo
}

# ---------- main -------------------------------------------------------------
header
require_root
require_deps
require_pve
configure
install_binary
cloud_login
install_service
verify_overseer
summary
