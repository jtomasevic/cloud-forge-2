#!/usr/bin/env bash
set -euo pipefail

MAX_RETRIES="${MAX_RETRIES:-30}"
RETRY_INTERVAL="${RETRY_INTERVAL:-3}"
SCYLLADB_HOSTS="${SCYLLADB_HOSTS:-localhost:9042}"
SCYLLADB_KEYSPACE="${SCYLLADB_KEYSPACE:-cloudforge}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
COMPOSE_FILE="${REPO_ROOT}/dev/docker-compose.yml"

run_cqlsh() {
	local statement="$1"

	if command -v cqlsh >/dev/null 2>&1; then
		cqlsh localhost 9042 -e "${statement}"
		return
	fi

	docker compose -f "${COMPOSE_FILE}" exec -T scylladb cqlsh scylladb 9042 -e "${statement}"
}

echo "Waiting for ScyllaDB to be ready..."
for i in $(seq 1 "${MAX_RETRIES}"); do
	if run_cqlsh "describe keyspaces" >/dev/null 2>&1; then
		echo "ScyllaDB is ready"
		break
	fi

	if [ "${i}" -eq "${MAX_RETRIES}" ]; then
		echo "ERROR: ScyllaDB did not become ready after ${MAX_RETRIES} attempts" >&2
		exit 1
	fi

	echo "  Attempt ${i}/${MAX_RETRIES} - waiting ${RETRY_INTERVAL}s..."
	sleep "${RETRY_INTERVAL}"
done

echo "Running schema migrations..."
cd "${REPO_ROOT}"
make migrate HOSTS="${SCYLLADB_HOSTS}" KEYSPACE="${SCYLLADB_KEYSPACE}"

echo "Schema migrations complete"
