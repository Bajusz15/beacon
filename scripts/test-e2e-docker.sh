#!/usr/bin/env bash
set -euo pipefail

###
# E2E test for Docker image polling using a local registry and tag updates.
# Requires Docker available on the host.
###

log_info()    { printf "[INFO] %s\n" "$*"; }
log_success() { printf "\033[32m[SUCCESS]\033[0m %s\n" "$*"; }
log_warn()    { printf "\033[33m[WARN]\033[0m %s\n" "$*"; }
log_error()   { printf "\033[31m[ERROR]\033[0m %s\n" "$*"; }

wait_for_docker() {
  local timeout="${1:-30}"
  local waited=0
  while [[ ${waited} -lt ${timeout} ]]; do
    if docker info >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done
  return 1
}

cleanup() {
  set +e
  if [[ -n "${DEPLOY_PID:-}" ]]; then
    kill "${DEPLOY_PID}" 2>/dev/null || true
    wait "${DEPLOY_PID}" 2>/dev/null || true
  fi
  docker rm -f beacon-e2e-registry 2>/dev/null || true
  rm -rf "${WORKDIR}" "${TMPDIR_DOCKER}"
}

trap cleanup EXIT

if ! command -v docker >/dev/null 2>&1; then
  log_error "Docker is required for this test"
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_NAME="e2e-docker"
WORKDIR="$(mktemp -d /tmp/beacon-e2e-docker-XXXXXX || mktemp -d)"
TMPDIR_DOCKER="$(mktemp -d /tmp/beacon-e2e-docker-tmp-XXXXXX || mktemp -d)"
CONFIG_DIR="${HOME}/.beacon/config/projects/${PROJECT_NAME}"
STATE_DIR="${HOME}/.beacon/${PROJECT_NAME}"
DEPLOY_LOG="${WORKDIR}/deploy.log"
BEACON_LOG="${WORKDIR}/beacon.log"
REGISTRY="${BEACON_E2E_REGISTRY:-localhost:5055}"
SKIP_REGISTRY_START="${BEACON_E2E_SKIP_REGISTRY_START:-0}"
IMAGE_BASE="${REGISTRY}/beacon-e2e/app"
TIMESTAMP="$(date +%s)"
# Use very high semantic versions so they sort above any leftover tags in the registry
TAG1="v9998.${TIMESTAMP}.0"
TAG2="v9999.${TIMESTAMP}.0"

log_info "Working directory: ${WORKDIR}"

mkdir -p "${CONFIG_DIR}" "${STATE_DIR}"

if [[ "${SKIP_REGISTRY_START}" != "1" ]]; then
  log_info "Starting local Docker registry at ${REGISTRY}..."
  REG_PORT="${REGISTRY##*:}"
  docker run -d --rm --name beacon-e2e-registry -p ${REG_PORT}:5000 registry:2 >/dev/null
else
  log_info "Skipping registry start; using existing registry at ${REGISTRY}"
fi

log_info "Waiting for Docker daemon to be ready..."
if wait_for_docker 45; then
  log_success "Docker daemon is ready"
else
  log_error "Docker daemon not ready within timeout"
  exit 1
fi

log_info "Pulling base image..."
docker pull busybox:latest >/dev/null

log_info "Pushing initial tag ${TAG1}..."
docker tag busybox:latest "${IMAGE_BASE}:${TAG1}"
docker push "${IMAGE_BASE}:${TAG1}" >/dev/null

log_info "Creating docker-images.yml..."
cat > "${CONFIG_DIR}/docker-images.yml" <<EOF
- image: "${IMAGE_BASE}"
  deploy_command: "echo \${BEACON_DOCKER_IMAGE} >> ${DEPLOY_LOG}"
EOF

# Ensure local path exists for beacon
mkdir -p "${WORKDIR}"

log_info "Starting beacon deploy in docker mode (poll interval 2s)..."
BEACON_DEPLOYMENT_TYPE="docker" \
BEACON_PROJECT_NAME="${PROJECT_NAME}" \
BEACON_LOCAL_PATH="${WORKDIR}" \
BEACON_POLL_INTERVAL="2s" \
beacon deploy > "${BEACON_LOG}" 2>&1 &
DEPLOY_PID=$!
sleep 2

wait_for_tag() {
  local tag="$1"
  local timeout="$2"
  local waited=0
  while [[ ${waited} -lt ${timeout} ]]; do
    if grep -q "${tag}" "${DEPLOY_LOG}" 2>/dev/null; then
      return 0
    fi
    if grep -q "${tag}" "${BEACON_LOG}" 2>/dev/null; then
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done
  return 1
}

log_info "Waiting for initial tag ${TAG1} to be deployed..."
if wait_for_tag "${TAG1}" 20; then
  log_success "Initial deploy for ${TAG1} detected"
else
  log_error "Initial deploy for ${TAG1} not detected"
  log_info "Beacon log:"
  tail -20 "${BEACON_LOG}" || true
  exit 1
fi

log_info "Pushing new tag ${TAG2}..."
docker tag busybox:latest "${IMAGE_BASE}:${TAG2}"
docker push "${IMAGE_BASE}:${TAG2}" >/dev/null

log_info "Waiting for new tag ${TAG2} to be detected and deployed..."
if wait_for_tag "${TAG2}" 30; then
  log_success "Tag update to ${TAG2} detected and deployed"
else
  log_error "Tag update to ${TAG2} not detected"
  log_info "Beacon log:"
  tail -40 "${BEACON_LOG}" || true
  exit 1
fi

log_success "Docker E2E test passed"

