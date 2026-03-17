#!/usr/bin/env bash
# E2E test: run a pod in Kubernetes that writes logs to a shared file; Beacon (sidecar) tails
# the file and forwards to a mock HTTP server. We verify the mock server received the POST.
# Requires: kind, kubectl, docker (to build images).
set -euo pipefail

MARKER="E2E_LOG_FORWARDING_MARKER"
CLUSTER_NAME="${CLUSTER_NAME:-beacon-log-e2e}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
E2E_DIR="${ROOT_DIR}/deploy/kubernetes/e2e"
TIMEOUT=120

log_info()    { printf "[INFO] %s\n" "$*"; }
log_success() { printf "\033[32m[SUCCESS]\033[0m %s\n" "$*"; }
log_error()   { printf "\033[31m[ERROR]\033[0m %s\n" "$*"; }

KUBECONFIG_FILE=""

cleanup() {
  set +e
  [[ -n "${KUBECONFIG_FILE}" && -f "${KUBECONFIG_FILE}" ]] && rm -f "${KUBECONFIG_FILE}"
  kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
}
trap cleanup EXIT

if ! command -v kind &>/dev/null; then
  log_error "kind is required. Install from https://kind.sigs.k8s.io/docs/user/quick-start/"
  exit 1
fi
if ! command -v kubectl &>/dev/null; then
  log_error "kubectl is required"
  exit 1
fi
if ! command -v docker &>/dev/null; then
  log_error "docker is required to build images"
  exit 1
fi

log_info "Building mock-log-server image..."
docker build -t mock-log-server:e2e "${ROOT_DIR}/tests/k8s/mock-log-server"
# Build beacon image (use e2e Dockerfile which has beacon at /usr/local/bin)
log_info "Building beacon image..."
docker build -t beacon:e2e -f "${ROOT_DIR}/tests/e2e/Dockerfile" "${ROOT_DIR}"

log_info "Creating kind cluster: ${CLUSTER_NAME}"
kind create cluster --name "${CLUSTER_NAME}" --wait 60s 2>/dev/null || true

KUBECONFIG_FILE="$(mktemp)"
kind get kubeconfig --name "${CLUSTER_NAME}" > "${KUBECONFIG_FILE}"
export KUBECONFIG="${KUBECONFIG_FILE}"
log_info "Loading images into kind..."
kind load docker-image mock-log-server:e2e --name "${CLUSTER_NAME}"
kind load docker-image beacon:e2e --name "${CLUSTER_NAME}"

log_info "Deploying mock-log-server..."
kubectl apply -f "${E2E_DIR}/mock-log-server.e2e.yaml"
kubectl rollout status deployment/mock-log-server --timeout=60s

log_info "Deploying beacon log forwarding pod..."
kubectl apply -f "${E2E_DIR}/beacon-log-forwarding.e2e.yaml"

log_info "Waiting for pod to be running and Beacon to forward logs (timeout ${TIMEOUT}s)..."
kubectl wait --for=condition=Ready pod -l app=beacon-log-forwarding-e2e --timeout=60s 2>/dev/null || true

deadline=$((SECONDS + TIMEOUT))
while [[ $SECONDS -lt $deadline ]]; do
  if kubectl logs deployment/mock-log-server 2>/dev/null | grep -q "RECEIVED_POST_AGENT_LOGS.*${MARKER}"; then
    log_success "Mock server received POST /agent/logs with expected log content"
    exit 0
  fi
  sleep 3
done

log_error "Timeout: mock server did not receive expected log content"
log_info "Mock server logs:"
kubectl logs deployment/mock-log-server --tail=50 2>/dev/null || true
log_info "Beacon pod logs:"
kubectl logs pod/beacon-log-forwarding-e2e -c beacon --tail=50 2>/dev/null || true
exit 1
