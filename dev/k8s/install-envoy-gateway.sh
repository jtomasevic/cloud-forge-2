#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=use-k3d-kubeconfig.sh
source "${SCRIPT_DIR}/use-k3d-kubeconfig.sh"

ENVOY_GW_VERSION="v1.0.2"

echo "Installing Envoy Gateway ${ENVOY_GW_VERSION}"

helm upgrade --install envoy-gateway oci://docker.io/envoyproxy/gateway-helm \
	--version "${ENVOY_GW_VERSION}" \
	--namespace envoy-gateway-system \
	--create-namespace \
	--wait

# Install the GatewayClass (idempotent)
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: cloudforge
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
EOF

echo "Envoy Gateway installed and GatewayClass 'cloudforge' created"
