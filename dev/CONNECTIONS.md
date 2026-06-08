# CloudForge Dev - Connection Reference

## Local dev loop
- Start full k3d + Envoy Gateway mode: `make dev`
- Start faster local Go mode: `make dev-local`
- Prepare full k3d + Envoy Gateway mode without starting Tilt: `make dev-setup`
- Reapply only Envoy Gateway routes: `make gateway-apply`
- Stop Tilt resources and backing services: `make dev-down`
- Delete the k3d cluster too: `make dev-kill`
- End-to-end smoke test: `dev/scripts/smoke-test.sh`

`make dev` and `make dev-local` clear stale Tilt listeners on `TILT_PORT`
(`10350` by default) before starting. Use `TILT_PORT=10351 make dev` if you
intentionally want a separate Tilt instance.

`make dev` also points Tilt at the `cloudforge-dev` k3d kubeconfig automatically,
and requires the k3d-managed local registry created by `make k3d-up`, so Tilt
can push local images to k3d instead of `ghcr.io`. If your cluster was created
before the local registry was added, recreate it once with `make k3d-down && make dev`.

## CloudForge services
- CF-Accounts: http://localhost:8081
- CF-Provisioner: http://localhost:8082
- CF-Router: http://localhost:8083
- CF-Router health: http://localhost:8083/health
- CloudForge Swagger UI: http://localhost:8090/swagger/
- CF-Router OpenAPI JSON: http://localhost:8090/openapi/cf-router.json
- CF-Accounts via CF-Router OpenAPI JSON: http://localhost:8090/openapi/cf-accounts.json
- CF-Provisioner via CF-Router OpenAPI JSON: http://localhost:8090/openapi/cf-provisioner.json
- Envoy Gateway API: http://api.cloudforge.local:18080
- Envoy Gateway Swagger UI: http://api.cloudforge.local:18080/swagger/

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

## Get a dev JWT through CloudForge login
```bash
curl -sf -X POST http://api.cloudforge.local:18080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"dev-user@cloudforge.io","password":"devpassword"}' \
  | jq -r .accessToken
```

For a newly signed-up account, use the email and password submitted to
`POST /v1/accounts` with `POST /v1/auth/login`; CF-Accounts creates the
matching Keycloak user during signup and returns a Keycloak access token from
the login endpoint.
