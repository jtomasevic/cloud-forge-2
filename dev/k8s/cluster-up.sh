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
: "${CF_K3D_RECREATE_ON_DRIFT:=1}"

cluster_exists() {
	k3d cluster list 2>/dev/null | awk 'NR > 1 {print $1}' | grep -Fxq "${CLUSTER_NAME}"
}

registry_exists() {
	local names
	names="$(k3d registry list 2>/dev/null | awk 'NR > 1 {print $1}' || true)"
	printf '%s\n' "${names}" | grep -Fxq "${CF_K3D_REGISTRY_NAME}" ||
		printf '%s\n' "${names}" | grep -Fxq "k3d-${CF_K3D_REGISTRY_NAME}"
}

delete_cloudforge_registry() {
	k3d registry delete "${CF_K3D_REGISTRY_NAME}" >/dev/null 2>&1 || true
	k3d registry delete "k3d-${CF_K3D_REGISTRY_NAME}" >/dev/null 2>&1 || true
	if command -v docker >/dev/null 2>&1; then
		docker rm -f "k3d-${CF_K3D_REGISTRY_NAME}" >/dev/null 2>&1 || true
	fi
}

local_registry_config_exists() {
	local kubeconfig="$1"
	kubectl --kubeconfig="${kubeconfig}" -n kube-public get configmap local-registry-hosting \
		-o jsonpath='{.data.localRegistryHosting\.v1}' 2>/dev/null |
		grep -q "${CF_K3D_REGISTRY_PORT}"
}

recreate_cluster() {
	local reason="$1"
	if [[ "${CF_K3D_RECREATE_ON_DRIFT}" != "1" ]]; then
		echo "ERROR: ${CLUSTER_NAME} exists but is not usable for local CloudForge development." >&2
		echo "       Reason: ${reason}" >&2
		echo "       Recreate it with: make k3d-down && make k3d-up" >&2
		echo "       Or allow automatic repair with CF_K3D_RECREATE_ON_DRIFT=1." >&2
		exit 1
	fi

	echo "Detected k3d drift: ${reason}"
	echo "Recreating disposable cluster ${CLUSTER_NAME} with the required local registry..."
	k3d cluster delete "${CLUSTER_NAME}" >/dev/null 2>&1 || true
	delete_cloudforge_registry
}

create_cluster() {
	delete_cloudforge_registry

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
}

if cluster_exists; then
	echo "Cluster ${CLUSTER_NAME} already exists — skipping creation"
	__chk_kcfg="$(mktemp)"
	trap "rm -f '${__chk_kcfg}'" EXIT
	if ! k3d kubeconfig write "${CLUSTER_NAME}" --output "${__chk_kcfg}" --overwrite >/dev/null 2>&1; then
		k3d kubeconfig get "${CLUSTER_NAME}" >"${__chk_kcfg}"
	fi
	if ! kubectl --kubeconfig="${__chk_kcfg}" cluster-info --request-timeout=8s &>/dev/null; then
		echo "ERROR: ${CLUSTER_NAME} is registered in k3d but the Kubernetes API is not reachable." >&2
		echo "       Configured server:" >&2
		kubectl --kubeconfig="${__chk_kcfg}" config view --minify -o jsonpath='{.clusters[0].cluster.server}{"\n"}' 2>/dev/null || true
		echo "       Common fix after Docker restarts or port drift: make k3d-down && make k3d-up" >&2
		recreate_cluster "Kubernetes API is not reachable"
	elif ! registry_exists; then
		recreate_cluster "required k3d local registry is missing"
	elif ! local_registry_config_exists "${__chk_kcfg}"; then
		recreate_cluster "local registry discovery ConfigMap is missing"
	else
		echo "Tip: optional — merge kubeconfig for your shell's kubectl: make k3d-kubeconfig"
		exit 0
	fi
fi

create_cluster
