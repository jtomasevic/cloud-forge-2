#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="cloudforge-dev"
: "${CF_K3D_REGISTRY_NAME:=cloudforge-dev-registry.localhost}"

delete_cloudforge_registry() {
	k3d registry delete "${CF_K3D_REGISTRY_NAME}" >/dev/null 2>&1 || true
	k3d registry delete "k3d-${CF_K3D_REGISTRY_NAME}" >/dev/null 2>&1 || true
	if command -v docker >/dev/null 2>&1; then
		docker rm -f "k3d-${CF_K3D_REGISTRY_NAME}" >/dev/null 2>&1 || true
	fi
}

if ! k3d cluster list 2>/dev/null | awk 'NR > 1 {print $1}' | grep -Fxq "${CLUSTER_NAME}"; then
	echo "Cluster ${CLUSTER_NAME} does not exist — nothing to delete"
	delete_cloudforge_registry
	exit 0
fi

k3d cluster delete "${CLUSTER_NAME}"
echo "Cluster ${CLUSTER_NAME} deleted"
delete_cloudforge_registry
echo "CloudForge k3d registry deleted if it existed"
