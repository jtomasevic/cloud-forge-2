#!/usr/bin/env bash
set -euo pipefail

OPENBAO_ADDR="${OPENBAO_ADDR:-http://localhost:8200}"
OPENBAO_TOKEN="${OPENBAO_TOKEN:-dev-root-token}"
VAULT_ADDR="${OPENBAO_ADDR}"
VAULT_TOKEN="${OPENBAO_TOKEN}"
BAO_ADDR="${OPENBAO_ADDR}"
BAO_TOKEN="${OPENBAO_TOKEN}"
MAX_RETRIES="${MAX_RETRIES:-30}"
RETRY_INTERVAL="${RETRY_INTERVAL:-2}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
COMPOSE_FILE="${REPO_ROOT}/dev/docker-compose.yml"

export VAULT_ADDR VAULT_TOKEN BAO_ADDR BAO_TOKEN

docker_openbao_cli() {
	docker compose -f "${COMPOSE_FILE}" exec -T \
		-e BAO_ADDR="http://127.0.0.1:8200" \
		-e BAO_TOKEN="${OPENBAO_TOKEN}" \
		-e VAULT_ADDR="http://127.0.0.1:8200" \
		-e VAULT_TOKEN="${OPENBAO_TOKEN}" \
		openbao bao "$@"
}

openbao_ready() {
	if command -v curl >/dev/null 2>&1 && curl -fsS "${OPENBAO_ADDR}/v1/sys/health" >/dev/null 2>&1; then
		return 0
	fi

	docker_openbao_cli status >/dev/null 2>&1
}

echo "Waiting for OpenBao to be ready at ${OPENBAO_ADDR}..."
for i in $(seq 1 "${MAX_RETRIES}"); do
	if openbao_ready; then
		echo "OpenBao is ready"
		break
	fi

	if [ "${i}" -eq "${MAX_RETRIES}" ]; then
		echo "ERROR: OpenBao did not become ready after ${MAX_RETRIES} attempts" >&2
		exit 1
	fi

	echo "  Attempt ${i}/${MAX_RETRIES} - waiting ${RETRY_INTERVAL}s..."
	sleep "${RETRY_INTERVAL}"
done

openbao_cli() {
	if docker compose -f "${COMPOSE_FILE}" ps openbao >/dev/null 2>&1; then
		docker_openbao_cli "$@"
		return
	fi

	if command -v vault >/dev/null 2>&1; then
		vault "$@"
		return
	fi

	if command -v bao >/dev/null 2>&1; then
		bao "$@"
		return
	fi

	echo "ERROR: OpenBao CLI not found and Docker Compose service is unavailable" >&2
	exit 1
}

echo "Initializing OpenBao..."

secret_mount_version="$(
	curl -fsS -H "X-Vault-Token: ${OPENBAO_TOKEN}" "${OPENBAO_ADDR}/v1/sys/mounts" |
		jq -r '.["secret/"].options.version // empty'
)"
if [ -z "${secret_mount_version}" ]; then
	openbao_cli secrets enable -path=secret -version=1 kv
elif [ "${secret_mount_version}" != "1" ]; then
	echo "Remounting secret/ as KV v1 for CloudForge dev paths (was KV v${secret_mount_version})"
	openbao_cli secrets disable secret
	openbao_cli secrets enable -path=secret -version=1 kv
else
	echo "KV v1 secrets engine already enabled"
fi

openbao_cli policy write cf-provisioner - <<'EOF'
path "secret/tenants/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
EOF

echo "OpenBao initialized"
