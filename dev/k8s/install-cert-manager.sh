#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=use-k3d-kubeconfig.sh
source "${SCRIPT_DIR}/use-k3d-kubeconfig.sh"

CERT_MANAGER_VERSION="1.14.4"

echo "Installing cert-manager ${CERT_MANAGER_VERSION}"

helm repo add jetstack https://charts.jetstack.io --force-update
helm repo update

helm upgrade --install cert-manager jetstack/cert-manager \
	--version "v${CERT_MANAGER_VERSION}" \
	--namespace cert-manager \
	--create-namespace \
	--set installCRDs=true \
	--wait

echo "cert-manager installed"
