# Source this file from other dev/k8s/ scripts (after `set -euo pipefail`) so kubectl/helm always
# talk to the k3d cluster — not a stale merged ~/.kube/config (wrong 0.0.0.0:PORT → connection refused).
#
# Usage:
#   SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
#   # shellcheck source=use-k3d-kubeconfig.sh
#   source "${SCRIPT_DIR}/use-k3d-kubeconfig.sh"

: "${K3D_CLUSTER_NAME:=cloudforge-dev}"

__cf_k3d_kcfg="$(mktemp)"
# Expand path when registering trap (single-quoted traps do not expand $vars at runtime).
trap "rm -f '${__cf_k3d_kcfg}'" EXIT

# Prefer `kubeconfig write` (same as merge to file): refreshes server URL from live k3d/docker state.
# Fall back to `get` if write fails (older k3d).
if ! k3d kubeconfig write "${K3D_CLUSTER_NAME}" --output "${__cf_k3d_kcfg}" --overwrite 2>/dev/null; then
	k3d kubeconfig get "${K3D_CLUSTER_NAME}" >"${__cf_k3d_kcfg}"
fi

export KUBECONFIG="${__cf_k3d_kcfg}"

# Avoid picking up a wrong context from the parent shell (Helm still honors some env combos).
unset HELM_KUBECONTEXT 2>/dev/null || true

# Force client-go / Helm to use this file (defensive; some setups still read default kubeconfig).
if ! command kubectl --kubeconfig="${KUBECONFIG}" cluster-info --request-timeout=8s &>/dev/null; then
	echo "ERROR: cannot reach Kubernetes API for cluster '${K3D_CLUSTER_NAME}'." >&2
	echo "       Configured server:" >&2
	command kubectl --kubeconfig="${KUBECONFIG}" config view --minify -o jsonpath='{.clusters[0].cluster.server}{"\n"}' 2>/dev/null || true
	echo "       Try: make k3d-down && make k3d-up && make k3d-install-deps" >&2
	echo "       Or ensure Docker is running and k3d cluster is healthy: k3d cluster list" >&2
	exit 1
fi

helm() {
	command helm --kubeconfig "${KUBECONFIG}" "$@"
}
kubectl() {
	command kubectl --kubeconfig "${KUBECONFIG}" "$@"
}
