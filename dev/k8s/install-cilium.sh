#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=use-k3d-kubeconfig.sh
source "${SCRIPT_DIR}/use-k3d-kubeconfig.sh"

CILIUM_VERSION="1.15.4"

echo "Installing Cilium ${CILIUM_VERSION}"

helm repo add cilium https://helm.cilium.io/ --force-update
helm repo update

helm upgrade --install cilium cilium/cilium \
	--version "${CILIUM_VERSION}" \
	--namespace kube-system \
	--set image.pullPolicy=IfNotPresent \
	--set ipam.mode=kubernetes \
	--set kubeProxyReplacement=false \
	--set operator.replicas=1 \
	--set policyAuditMode=true \
	--set hubble.enabled=true \
	--set hubble.relay.enabled=true \
	--wait

echo "Cilium installed. Verifying..."
kubectl rollout status daemonset/cilium -n kube-system --timeout=120s
echo "Cilium ready"
