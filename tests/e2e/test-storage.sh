#!/usr/bin/env bash
set -euo pipefail

###
# E2E test for the Beacon storage gateway using a Docker-mounted NAS fixture.
# The fixture models the MVP contract: a NAS/share is already mounted on the
# Beacon host, and Beacon exposes only configured read-only paths.
###

log_info()    { printf "[INFO] %s\n" "$*"; }
log_success() { printf "\033[32m[SUCCESS]\033[0m %s\n" "$*"; }
log_error()   { printf "\033[31m[ERROR]\033[0m %s\n" "$*"; }

NAS_ROOT="${BEACON_E2E_NAS_ROOT:-/mnt/nas/media}"
NAS_WEBDAV_URL="${BEACON_E2E_NAS_WEBDAV_URL:-}"
HOME_DIR="${HOME:-/root}"
export HOME="${HOME_DIR}"
export BEACON_HOME="${BEACON_HOME:-${HOME_DIR}/.beacon-storage-e2e}"

BEACON_PORT="${BEACON_E2E_STORAGE_PORT:-9100}"
BEACON_LOG="/tmp/beacon-e2e-storage.log"

cleanup() {
  set +e
  if [[ -n "${BEACON_PID:-}" ]]; then
    kill "${BEACON_PID}" 2>/dev/null || true
    wait "${BEACON_PID}" 2>/dev/null || true
  fi
  rm -rf "${BEACON_HOME}"
  rm -f "${BEACON_LOG}" /tmp/beacon-e2e-storage-body /tmp/beacon-e2e-storage-headers
}

trap cleanup EXIT

wait_for_file() {
  local path="$1"
  local timeout="${2:-30}"
  local waited=0
  while [[ ${waited} -lt ${timeout} ]]; do
    if [[ -e "${path}" ]]; then
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done
  return 1
}

wait_for_http() {
  local url="$1"
  local timeout="${2:-30}"
  local waited=0
  while [[ ${waited} -lt ${timeout} ]]; do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done
  return 1
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"
  if ! grep -q "${needle}" <<<"${haystack}"; then
    log_error "${label}: expected to find ${needle}"
    printf "%s\n" "${haystack}"
    exit 1
  fi
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"
  if grep -q "${needle}" <<<"${haystack}"; then
    log_error "${label}: did not expect to find ${needle}"
    printf "%s\n" "${haystack}"
    exit 1
  fi
}

http_status() {
  curl -sS -o /tmp/beacon-e2e-storage-body -w "%{http_code}" "$@"
}

log_info "Starting storage gateway E2E"
log_info "Beacon home: ${BEACON_HOME}"
log_info "NAS root: ${NAS_ROOT}"
if [[ -n "${NAS_WEBDAV_URL}" ]]; then
  log_info "NAS WebDAV URL: ${NAS_WEBDAV_URL}"
fi

if ! command -v beacon >/dev/null 2>&1; then
  log_error "Beacon binary not found in PATH"
  exit 1
fi
if ! command -v curl >/dev/null 2>&1; then
  log_error "curl is required"
  exit 1
fi

log_info "Waiting for NAS fixture..."
if ! wait_for_file "${NAS_ROOT}/clip.txt" 30; then
  log_error "NAS fixture did not appear at ${NAS_ROOT}"
  ls -la "$(dirname "${NAS_ROOT}")" 2>/dev/null || true
  exit 1
fi
if [[ -n "${NAS_WEBDAV_URL}" ]]; then
  log_info "Checking NAS service directly..."
  if ! wait_for_http "${NAS_WEBDAV_URL}/clip.txt" 30; then
    log_error "NAS WebDAV service did not serve clip.txt"
    exit 1
  fi
  nas_body="$(curl -fsS "${NAS_WEBDAV_URL}/clip.txt")"
  if [[ "${nas_body}" != "0123456789" ]]; then
    log_error "NAS WebDAV service returned unexpected body"
    printf "%s\n" "${nas_body}"
    exit 1
  fi
fi

rm -rf "${BEACON_HOME}"
mkdir -p "${BEACON_HOME}"

log_info "Initializing Beacon and adding storage share..."
beacon init --name e2e-storage-device --metrics-port "${BEACON_PORT}" >/dev/null
beacon storage add media --root "${NAS_ROOT}" --name "NAS Media" >/dev/null

storage_list="$(beacon storage list)"
assert_contains "${storage_list}" "media" "storage list"
assert_contains "${storage_list}" "${NAS_ROOT}" "storage list"
assert_contains "${storage_list}" "true" "storage list"

log_info "Starting Beacon master..."
beacon start --foreground >"${BEACON_LOG}" 2>&1 &
BEACON_PID=$!

if ! wait_for_http "http://127.0.0.1:${BEACON_PORT}/health" 30; then
  log_error "Beacon did not start"
  tail -80 "${BEACON_LOG}" || true
  exit 1
fi

log_info "Checking storage share catalog..."
shares="$(curl -fsS "http://127.0.0.1:${BEACON_PORT}/api/storage/shares")"
assert_contains "${shares}" '"id":"media"' "storage shares"
assert_contains "${shares}" '"name":"NAS Media"' "storage shares"
assert_not_contains "${shares}" "${NAS_ROOT}" "storage shares"

log_info "Checking directory listing and metadata..."
listing="$(curl -fsS "http://127.0.0.1:${BEACON_PORT}/api/storage/shares/media/list")"
assert_contains "${listing}" '"name":"docs"' "storage listing"
assert_contains "${listing}" '"type":"directory"' "storage listing"
assert_contains "${listing}" '"name":"clip.txt"' "storage listing"

metadata="$(curl -fsS --get "http://127.0.0.1:${BEACON_PORT}/api/storage/shares/media/metadata" --data-urlencode "path=docs/readme.txt")"
assert_contains "${metadata}" '"name":"readme.txt"' "storage metadata"
assert_contains "${metadata}" '"type":"file"' "storage metadata"

log_info "Checking Range download support..."
curl -fsS -D /tmp/beacon-e2e-storage-headers \
  -H "Range: bytes=2-5" \
  --get "http://127.0.0.1:${BEACON_PORT}/api/storage/shares/media/download" \
  --data-urlencode "path=clip.txt" \
  -o /tmp/beacon-e2e-storage-body
if [[ "$(cat /tmp/beacon-e2e-storage-body)" != "2345" ]]; then
  log_error "Range download returned unexpected body"
  cat /tmp/beacon-e2e-storage-body
  exit 1
fi
if ! grep -qi '^HTTP/.* 206' /tmp/beacon-e2e-storage-headers; then
  log_error "Range download did not return HTTP 206"
  cat /tmp/beacon-e2e-storage-headers
  exit 1
fi
if ! grep -qi '^Content-Range: bytes 2-5/10' /tmp/beacon-e2e-storage-headers; then
  log_error "Range download returned unexpected Content-Range"
  cat /tmp/beacon-e2e-storage-headers
  exit 1
fi

log_info "Checking path escape protection..."
status="$(http_status --get "http://127.0.0.1:${BEACON_PORT}/api/storage/shares/media/metadata" --data-urlencode "path=../secret.txt")"
if [[ "${status}" != "403" ]]; then
  log_error "Traversal path returned HTTP ${status}, expected 403"
  cat /tmp/beacon-e2e-storage-body
  exit 1
fi

status="$(http_status --get "http://127.0.0.1:${BEACON_PORT}/api/storage/shares/media/metadata" --data-urlencode "path=escape-host")"
if [[ "${status}" != "403" ]]; then
  log_error "Symlink escape returned HTTP ${status}, expected 403"
  cat /tmp/beacon-e2e-storage-body
  exit 1
fi

audit_log="$(curl -fsS "http://127.0.0.1:${BEACON_PORT}/api/audit?action=storage_download")"
assert_contains "${audit_log}" "storage_download" "storage audit"
assert_not_contains "${audit_log}" "clip.txt" "storage audit"

log_success "Storage gateway Docker E2E test passed"
