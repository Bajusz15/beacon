#!/usr/bin/env bash
set -euo pipefail

###
# E2E test for Beacon MCP server (HTTP transport).
# Requires: beacon binary (or builds it), go.
###

log_info()    { printf "[INFO] %s\n" "$*"; }
log_success() { printf "\033[32m[SUCCESS]\033[0m %s\n" "$*"; }
log_error()   { printf "\033[31m[ERROR]\033[0m %s\n" "$*"; }

MCP_PORT="${BEACON_MCP_PORT:-7767}"
MCP_ADDR="127.0.0.1:${MCP_PORT}"
MCP_URL="http://${MCP_ADDR}"

cleanup() {
  set +e
  if [[ -n "${MCP_PID:-}" ]]; then
    kill "${MCP_PID}" 2>/dev/null || true
    wait "${MCP_PID}" 2>/dev/null || true
  fi
  rm -rf "${E2E_HOME:-}"
}

trap cleanup EXIT

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

E2E_HOME="$(mktemp -d /tmp/beacon-e2e-mcp-XXXXXX)"
BEACON_DIR="${E2E_HOME}/.beacon"
mkdir -p "${BEACON_DIR}/config/projects/e2e-mcp-proj"
mkdir -p "${BEACON_DIR}/state/e2e-mcp-proj"
mkdir -p "${BEACON_DIR}/logs/e2e-mcp-proj"
mkdir -p "${BEACON_DIR}/templates"

log_info "Bootstrap test project at ${BEACON_DIR}"
PROJ_LOC="${E2E_HOME}/beacon/e2e-mcp-proj"
echo '{"projects":[{"name":"e2e-mcp-proj","location":"'${PROJ_LOC}'","config_dir":"'${BEACON_DIR}'/config/projects/e2e-mcp-proj"}]}' > "${BEACON_DIR}/projects.json"
mkdir -p "${PROJ_LOC}"

echo '{"updated_at":"2024-01-01T00:00:00Z","checks":[{"name":"http","status":"up","timestamp":"2024-01-01T00:00:00Z"},{"name":"ping","status":"down","timestamp":"2024-01-01T00:00:00Z","error":"timeout"}]}' > "${BEACON_DIR}/state/e2e-mcp-proj/checks.json"
echo -e "2024-01-01T00:00:00Z [deploy] Starting\n2024-01-01T00:00:01Z [deploy] Done" > "${BEACON_DIR}/logs/e2e-mcp-proj/deploy.log"

log_info "Creating git repo for beacon_diff..."
(cd "${PROJ_LOC}" && git init -q && echo "v1" > file.txt && git add file.txt && git commit -q -m "v1" && git tag v1.0.0 && echo "v2" >> file.txt && git add file.txt && git commit -q -m "v2" && git tag v2.0.0)

BEACON_BIN="${E2E_HOME}/beacon-bin"
log_info "Building beacon..."
if ! go build -o "${BEACON_BIN}" ./cmd/beacon; then
  log_error "Failed to build beacon"
  exit 1
fi

log_info "Starting MCP server on ${MCP_URL}..."
HOME="${E2E_HOME}" "${BEACON_BIN}" mcp serve --transport http --listen "${MCP_ADDR}" &
MCP_PID=$!

wait_for_mcp() {
  local timeout="${1:-15}"
  local waited=0
  while [[ ${waited} -lt ${timeout} ]]; do
    if curl -sf -X POST "${MCP_URL}" -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"1.0"}}}' >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done
  return 1
}

if ! wait_for_mcp; then
  log_error "MCP server did not become reachable within 15s"
  exit 1
fi

log_info "Running E2E tests..."
export BEACON_MCP_ENDPOINT="${MCP_URL}"
if ! go test ./scripts/test-e2e-mcp/ -run TestE2EMCPTools -count=1 -v; then
  log_error "E2E tests failed"
  exit 1
fi

log_success "MCP E2E test passed"
