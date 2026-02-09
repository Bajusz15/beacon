#!/usr/bin/env bash
# Create a kind cluster and run beacon source status against it for smoke testing.
# Requires: kind, kubectl, go (to build beacon).
set -e

CLUSTER_NAME="${CLUSTER_NAME:-beacon-observer-test}"
PROJECT="${PROJECT:-k8s-test}"
BEACON_BIN="${BEACON_BIN:-}"

if ! command -v kind &>/dev/null; then
  echo "kind not found. Install from https://kind.sigs.k8s.io/docs/user/quick-start/"
  exit 1
fi

if ! command -v kubectl &>/dev/null; then
  echo "kubectl not found. Install kubectl."
  exit 1
fi

if [[ -z "$BEACON_BIN" ]]; then
  echo "Building beacon..."
  (cd "$(dirname "$0")/.." && go build -o /tmp/beacon-k8s-test ./cmd/beacon)
  BEACON_BIN=/tmp/beacon-k8s-test
fi

echo "Creating kind cluster: $CLUSTER_NAME"
kind create cluster --name "$CLUSTER_NAME" --wait 60s 2>/dev/null || true

export KUBECONFIG="$(kind get kubeconfig-path --name "$CLUSTER_NAME" 2>/dev/null || kind get kubeconfig --name "$CLUSTER_NAME" >/tmp/kind-kubeconfig && echo /tmp/kind-kubeconfig)"
echo "KUBECONFIG=$KUBECONFIG"

# Deploy a simple deployment so we have something to observe
kubectl create namespace default 2>/dev/null || true
kubectl run nginx --image=nginx:alpine --restart=Never 2>/dev/null || true
kubectl run busybox --image=busybox:1.36 --restart=Never -- sleep 3600 2>/dev/null || true

echo "Adding Kubernetes source and running status..."
"$BEACON_BIN" source add kubernetes kind-source --project "$PROJECT" --kubeconfig "$KUBECONFIG" --namespace default
"$BEACON_BIN" source status kind-source --project "$PROJECT" --timeout 45s

echo "Done. To delete cluster: kind delete cluster --name $CLUSTER_NAME"
