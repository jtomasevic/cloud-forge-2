#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="cloudforge-dev"

if ! k3d cluster list 2>/dev/null | grep -q "${CLUSTER_NAME}"; then
	echo "Cluster ${CLUSTER_NAME} does not exist — nothing to delete"
	exit 0
fi

k3d cluster delete "${CLUSTER_NAME}"
echo "Cluster ${CLUSTER_NAME} deleted"
