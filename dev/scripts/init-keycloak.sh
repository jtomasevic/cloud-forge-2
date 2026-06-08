#!/usr/bin/env bash
set -euo pipefail

KEYCLOAK_ADMIN_URL="${KEYCLOAK_ADMIN_URL:-http://localhost:8084/auth}"
KEYCLOAK_REALM="${KEYCLOAK_REALM:-cloudforge}"
KEYCLOAK_ADMIN_REALM="${KEYCLOAK_ADMIN_REALM:-master}"
KEYCLOAK_ADMIN_CLIENT_ID="${KEYCLOAK_ADMIN_CLIENT_ID:-admin-cli}"
KEYCLOAK_ADMIN_USERNAME="${KEYCLOAK_ADMIN_USERNAME:-admin}"
KEYCLOAK_ADMIN_PASSWORD="${KEYCLOAK_ADMIN_PASSWORD:-admin}"
MAX_RETRIES="${MAX_RETRIES:-30}"
RETRY_INTERVAL="${RETRY_INTERVAL:-2}"

require_tool() {
	local tool="$1"
	if ! command -v "${tool}" >/dev/null 2>&1; then
		echo "ERROR: ${tool} is required to initialize Keycloak" >&2
		exit 127
	fi
}

require_tool curl
require_tool jq

echo "Waiting for Keycloak to be ready at ${KEYCLOAK_ADMIN_URL}..."
for i in $(seq 1 "${MAX_RETRIES}"); do
	if curl -fsS "${KEYCLOAK_ADMIN_URL}/health/ready" >/dev/null 2>&1; then
		echo "Keycloak is ready"
		break
	fi

	if [ "${i}" -eq "${MAX_RETRIES}" ]; then
		echo "ERROR: Keycloak did not become ready after ${MAX_RETRIES} attempts" >&2
		exit 1
	fi

	echo "  Attempt ${i}/${MAX_RETRIES} - waiting ${RETRY_INTERVAL}s..."
	sleep "${RETRY_INTERVAL}"
done

echo "Initializing Keycloak..."

admin_token="$(
	curl -fsS -X POST "${KEYCLOAK_ADMIN_URL}/realms/${KEYCLOAK_ADMIN_REALM}/protocol/openid-connect/token" \
		-d "grant_type=password" \
		-d "client_id=${KEYCLOAK_ADMIN_CLIENT_ID}" \
		-d "username=${KEYCLOAK_ADMIN_USERNAME}" \
		-d "password=${KEYCLOAK_ADMIN_PASSWORD}" |
		jq -er '.access_token'
)"

profile_file="$(mktemp)"
updated_profile_file="$(mktemp)"
trap 'rm -f "${profile_file}" "${updated_profile_file}"' EXIT

curl -fsS \
	-H "Authorization: Bearer ${admin_token}" \
	"${KEYCLOAK_ADMIN_URL}/admin/realms/${KEYCLOAK_REALM}/users/profile" \
	>"${profile_file}"

jq '
	.attributes = (
		(.attributes // [])
		| map(select(.name != "cf_account_id"))
		+ [{
			"name": "cf_account_id",
			"displayName": "CloudForge account ID",
			"validations": {
				"length": {
					"max": 64
				}
			},
			"permissions": {
				"view": ["admin"],
				"edit": ["admin"]
			},
			"multivalued": false
		}]
	)
' "${profile_file}" >"${updated_profile_file}"

curl -fsS -X PUT \
	-H "Authorization: Bearer ${admin_token}" \
	-H "Content-Type: application/json" \
	--data-binary @"${updated_profile_file}" \
	"${KEYCLOAK_ADMIN_URL}/admin/realms/${KEYCLOAK_REALM}/users/profile" \
	>/dev/null

echo "Keycloak initialized"
