# CloudForge Dev - Connection Reference

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
