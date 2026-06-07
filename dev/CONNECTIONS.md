# CloudForge Dev - Connection Reference

## Local dev loop
- Setup: `make dev-setup`
- Start local mode: `make tilt-up`
- Start k3d mode: `CF_DEV_MODE=k8s make tilt-up`
- Stop Tilt resources: `make tilt-down`
- End-to-end smoke test: `dev/scripts/smoke-test.sh`

## CloudForge services
- CF-Accounts: http://localhost:8081
- CF-Provisioner: http://localhost:8082
- CF-Router: http://localhost:8083
- CF-Router health: http://localhost:8083/health
- CloudForge Swagger UI: http://localhost:8090/swagger/
- CF-Router OpenAPI JSON: http://localhost:8090/openapi/cf-router.json
- CF-Accounts via CF-Router OpenAPI JSON: http://localhost:8090/openapi/cf-accounts.json
- CF-Provisioner via CF-Router OpenAPI JSON: http://localhost:8090/openapi/cf-provisioner.json
- Envoy Gateway via k3d LB: http://localhost:18080

Use CF-Router for external API calls. CF-Provisioner routes require
`X-CF-Internal-Secret: dev-internal-secret` when called directly.

## ScyllaDB
- CQL: `cqlsh localhost 9042`
- Docker CQL: `docker compose -f dev/docker-compose.yml exec -T scylladb cqlsh scylladb 9042`
- Keyspace: `cloudforge`

## OpenBao (Vault-compatible)
- UI: http://localhost:8200/ui
- Token: `dev-root-token`
- CLI: `VAULT_ADDR=http://localhost:8200 VAULT_TOKEN=dev-root-token vault kv list secret/tenants/`

## Keycloak
- Admin console: http://localhost:8084/auth/admin
- Health: http://localhost:8084/auth/health/ready
- Admin credentials: `admin` / `admin`
- Dev realm: `cloudforge`
- JWKS URL: http://localhost:8084/auth/realms/cloudforge/protocol/openid-connect/certs
- Token URL: http://localhost:8084/auth/realms/cloudforge/protocol/openid-connect/token

## Get a dev JWT (for testing CF-Router)
```bash
curl -X POST http://localhost:8084/auth/realms/cloudforge/protocol/openid-connect/token \
  -d "grant_type=password&client_id=cf-console&username=dev-user@cloudforge.io&password=devpassword" \
  | jq -r .access_token
```
