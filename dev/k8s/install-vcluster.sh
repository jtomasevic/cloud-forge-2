#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=use-k3d-kubeconfig.sh
source "${SCRIPT_DIR}/use-k3d-kubeconfig.sh"

VCLUSTER_VERSION="0.19.5"

echo "Installing vCluster Helm chart (operator) ${VCLUSTER_VERSION}"

helm repo add loft-sh https://charts.loft.sh --force-update
helm repo update

# Chart installs operator/CRDs — actual vCluster instances are created by CF-Provisioner.
helm upgrade --install vcluster-operator loft-sh/vcluster-k8s \
	--version "${VCLUSTER_VERSION}" \
	--namespace vcluster-system \
	--create-namespace \
	--set "vcluster.image=ghcr.io/loft-sh/vcluster:${VCLUSTER_VERSION}" \
	--wait

echo "vCluster operator/CRDs installed"
