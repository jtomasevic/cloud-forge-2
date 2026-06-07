#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${CF_ROUTER_URL:-http://localhost:8083}"
CF_ACCOUNTS_URL="${CF_ACCOUNTS_URL:-http://localhost:8081}"
KEYCLOAK_TOKEN_URL="${KEYCLOAK_TOKEN_URL:-http://localhost:8084/auth/realms/cloudforge/protocol/openid-connect/token}"

DEV_ACCOUNT_ID="11111111-1111-4111-8111-111111111111"
DEV_TENANT_ID="33333333-3333-4333-8333-333333333333"
DEV_NETWORK_ID="44444444-4444-4444-8444-444444444444"
SEED_TIME="2024-01-01T00:00:00Z"

REPO_ROOT="$(git rev-parse --show-toplevel)"
COMPOSE_FILE="${REPO_ROOT}/dev/docker-compose.yml"

run_cql() {
	local statement="$1"

	if command -v cqlsh >/dev/null 2>&1; then
		cqlsh localhost 9042 -e "${statement}"
		return
	fi

	docker compose -f "${COMPOSE_FILE}" exec -T scylladb cqlsh scylladb 9042 -e "${statement}"
}

json_value() {
	local expr="$1"
	python3 -c "import sys,json; data=json.load(sys.stdin); print(${expr})"
}

echo "=== CloudForge Dev Smoke Test ==="

echo "1. CF-Router health check..."
curl -sf "${BASE_URL}/health" | grep '"status":"ok"' >/dev/null
echo "   PASS"

echo "2. Seed dev tenant context..."
run_cql "INSERT INTO cloudforge.accounts (id, email, status, password_hash, created_at, updated_at) VALUES (${DEV_ACCOUNT_ID}, 'dev-user@cloudforge.io', 'active', 'dev-seed', '${SEED_TIME}', '${SEED_TIME}');"
run_cql "INSERT INTO cloudforge.accounts_by_email (email, account_id, status) VALUES ('dev-user@cloudforge.io', ${DEV_ACCOUNT_ID}, 'active');"
run_cql "INSERT INTO cloudforge.tenants (id, account_id, slug, region, status, created_at, updated_at) VALUES (${DEV_TENANT_ID}, ${DEV_ACCOUNT_ID}, 'dev-user', 'local', 'active', '${SEED_TIME}', '${SEED_TIME}');"
run_cql "INSERT INTO cloudforge.tenants_by_account (account_id, tenant_id, slug, status, created_at) VALUES (${DEV_ACCOUNT_ID}, ${DEV_TENANT_ID}, 'dev-user', 'active', '${SEED_TIME}');"
run_cql "INSERT INTO cloudforge.tenants_by_slug (slug, tenant_id, account_id, status) VALUES ('dev-user', ${DEV_TENANT_ID}, ${DEV_ACCOUNT_ID}, 'active');"
run_cql "INSERT INTO cloudforge.networks (id, tenant_id, region, pod_cidr, svc_cidr, status, created_at, updated_at) VALUES (${DEV_NETWORK_ID}, ${DEV_TENANT_ID}, 'local', '10.244.0.0/16', '10.96.0.0/12', 'active', '${SEED_TIME}', '${SEED_TIME}');"
run_cql "INSERT INTO cloudforge.networks_by_tenant (tenant_id, network_id, region, status, created_at) VALUES (${DEV_TENANT_ID}, ${DEV_NETWORK_ID}, 'local', 'active', '${SEED_TIME}');"
echo "   PASS"

echo "3. Get dev JWT..."
TOKEN="$(curl -sf -X POST "${KEYCLOAK_TOKEN_URL}" \
	-d "grant_type=password&client_id=cf-console&username=dev-user@cloudforge.io&password=devpassword" \
	| json_value "data['access_token']")"
echo "   PASS"

echo "4. Create account via CF-Router..."
SMOKE_EMAIL="smoke-test-$(date +%s)@example.com"
ACCOUNT="$(curl -sf -X POST "${BASE_URL}/v1/accounts" \
	-H "Authorization: Bearer ${TOKEN}" \
	-H "Content-Type: application/json" \
	-d "{\"email\":\"${SMOKE_EMAIL}\",\"password\":\"smokepassword1\"}")"
ACCOUNT_ID="$(echo "${ACCOUNT}" | json_value "data['account']['id']")"
echo "   PASS (account ID: ${ACCOUNT_ID})"

echo "5. Get account via CF-Router..."
curl -sf "${BASE_URL}/v1/accounts/${ACCOUNT_ID}" \
	-H "Authorization: Bearer ${TOKEN}" | grep '"status":"active"' >/dev/null
echo "   PASS"

echo "6. CF-Accounts direct readiness..."
curl -sf "${CF_ACCOUNTS_URL}/v1/accounts?limit=1" >/dev/null
echo "   PASS"

echo "=== Smoke test passed ==="
