#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="cloudforge-dev"
CLUSTER_API_PORT=6550
# Published onto the k3d server load balancer (host → LB 80/443). Defaults avoid 8080/8443,
# which often collide with Keycloak, other local stacks, or a second k3d cluster.
: "${CF_K3D_LB_HTTP_PORT:=18080}"
: "${CF_K3D_LB_HTTPS_PORT:=18443}"
: "${CF_K3D_REGISTRY_NAME:=cloudforge-dev-registry.localhost}"
: "${CF_K3D_REGISTRY_HOST:=127.0.0.1}"
: "${CF_K3D_REGISTRY_PORT:=5001}"

if k3d cluster list 2>/dev/null | grep -q "${CLUSTER_NAME}"; then
	echo "Cluster ${CLUSTER_NAME} already exists — skipping creation"
	__chk_kcfg="$(mktemp)"
	trap "rm -f '${__chk_kcfg}'" EXIT
	if ! k3d kubeconfig write "${CLUSTER_NAME}" --output "${__chk_kcfg}" --overwrite 2>/dev/null; then
		k3d kubeconfig get "${CLUSTER_NAME}" >"${__chk_kcfg}"
	fi
	if ! kubectl --kubeconfig="${__chk_kcfg}" cluster-info --request-timeout=8s &>/dev/null; then
		echo "ERROR: ${CLUSTER_NAME} is registered in k3d but the Kubernetes API is not reachable." >&2
		echo "       Configured server:" >&2
		kubectl --kubeconfig="${__chk_kcfg}" config view --minify -o jsonpath='{.clusters[0].cluster.server}{"\n"}' 2>/dev/null || true
		echo "       Common fix after Docker restarts or port drift: make k3d-down && make k3d-up" >&2
		exit 1
	fi
	if ! k3d registry list 2>/dev/null | grep -q "k3d-${CF_K3D_REGISTRY_NAME}"; then
		echo "ERROR: ${CLUSTER_NAME} exists without the required k3d local registry." >&2
		echo "       Tilt needs this registry to load local dev images without pushing to ghcr.io." >&2
		echo "       Recreate the disposable dev cluster once: make k3d-down && make dev" >&2
		exit 1
	fi
	echo "Tip: optional — merge kubeconfig for your shell's kubectl: make k3d-kubeconfig"
	exit 0
fi

echo "Creating k3d cluster: ${CLUSTER_NAME}"
k3d cluster create "${CLUSTER_NAME}" \
	--api-port "${CLUSTER_API_PORT}" \
	--registry-create "${CF_K3D_REGISTRY_NAME}:${CF_K3D_REGISTRY_HOST}:${CF_K3D_REGISTRY_PORT}" \
	--port "${CF_K3D_LB_HTTP_PORT}:80@loadbalancer" \
	--port "${CF_K3D_LB_HTTPS_PORT}:443@loadbalancer" \
	--k3s-arg "--disable=traefik@server:0" \
	--k3s-arg "--flannel-backend=none@server:0" \
	--k3s-arg "--disable-network-policy@server:0" \
	--wait

echo "Cluster ${CLUSTER_NAME} created successfully"
echo "Edge (Envoy Gateway once installed): http://127.0.0.1:${CF_K3D_LB_HTTP_PORT}  https://127.0.0.1:${CF_K3D_LB_HTTPS_PORT}"
echo "Local image registry: ${CF_K3D_REGISTRY_NAME}:${CF_K3D_REGISTRY_PORT}"
echo "Override host ports next time: CF_K3D_LB_HTTP_PORT CF_K3D_LB_HTTPS_PORT"
echo "KUBECONFIG: $(k3d kubeconfig get "${CLUSTER_NAME}")"
