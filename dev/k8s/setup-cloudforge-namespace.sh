#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=use-k3d-kubeconfig.sh
source "${SCRIPT_DIR}/use-k3d-kubeconfig.sh"

kubectl create namespace cloudforge-control-plane --dry-run=client -o yaml | kubectl apply -f -

# Self-signed ClusterIssuer for dev TLS (idempotent)
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: cloudforge-selfsigned
spec:
  selfSigned: {}
EOF

echo "Namespace and ClusterIssuer created"
