#!/usr/bin/env bash
# Setup script for bringing up the MySQL operator dev environment on a new machine.
# Run from repo root: ./scripts/setup.sh

set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-mysql-operator-dev}"
cd "$REPO_ROOT"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()   { echo -e "${RED}[ERR]${NC} $*"; exit 1; }

check_cmd() {
  if command -v "$1" &>/dev/null; then
    info "$1: $(command -v "$1")"
    return 0
  fi
  err "$1 is not installed. See SETUP.md for install instructions."
}

# --- Prerequisites ---
info "Checking prerequisites..."
check_cmd go
check_cmd kubectl
check_cmd kind

if ! docker info &>/dev/null; then
  err "Docker is not running. Start Colima/Docker Desktop and run this script again."
fi
info "Docker is reachable."

GO_VER=$(go version | sed -n 's/.*go\([0-9]*\.[0-9]*\).*/\1/p')
if [ -n "$GO_VER" ]; then
  info "Go version: $GO_VER (need 1.24+)"
fi

# --- Kind cluster ---
if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
  info "Kind cluster '$KIND_CLUSTER_NAME' already exists."
else
  info "Creating Kind cluster '$KIND_CLUSTER_NAME'..."
  kind create cluster --name "$KIND_CLUSTER_NAME"
fi

kubectl config use-context "kind-${KIND_CLUSTER_NAME}" || true
kubectl cluster-info
kubectl get nodes

# --- Project deps & CRDs ---
info "Installing CRDs (make install)..."
make install

if ! kubectl get crds 2>/dev/null | grep -q mysqls.database.mycompany.com; then
  err "CRD install may have failed. Check 'make install' output."
fi
info "CRDs installed."

# --- Go mod ---
info "Ensuring Go modules are downloaded..."
go mod download

info ""
info "Setup complete. Next steps:"
info "  1. In one terminal, run:  make run"
info "  2. In another terminal:  kubectl apply -f config/samples/mysql-8.4.x.yaml"
info "  3. Watch status:         kubectl get mysql my-mysql-8.4.x -w"
info ""
info "To tear down:  kind delete cluster --name $KIND_CLUSTER_NAME"
