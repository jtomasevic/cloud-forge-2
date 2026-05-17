# CF-Accounts

HTTP API service for CloudForge **accounts**, **tenants**, **networks**, **API credentials**, and **internal tenant resolution** (CF-Router). It sits on **ScyllaDB** (see `tools/migrations/scripts/`) and exposes routes defined in [`api/cf-accounts/v1/openapi.yaml`](../../api/cf-accounts/v1/openapi.yaml).

## Features (service + HTTP)

- **Signup** — `POST /v1/accounts` accepts `email` and `password`. Passwords are stored as **bcrypt** hashes only (`password_hash` column). The response is **`CreateAccountResult`**: `account` plus **`defaultTenant`** (id, slug, `provisioning`) so clients do not need an extra list call.
- **Login** — `POST /v1/auth/login` (no Bearer auth on this route). Password must meet the same **minimum length (8)** as signup (OpenAPI + service). Invalid password shape returns **400** (`INVALID_INPUT`); wrong email/password for an otherwise valid request returns **401** (`UNAUTHORIZED`) without distinguishing unknown email from bad password.
- **OpenAPI** — Server stubs live under `internal/rest/generated/` (`go generate ./...` from this module). The REST layer maps generated DTOs ↔ service models in `internal/rest/models_transform.go`.

## Run locally

Prerequisites: Scylla (or Cassandra-compatible) reachable with keyspace and tables applied (`tools/migrations`).

```bash
export SCYLLADB_HOSTS=127.0.0.1:9042   # optional, default localhost:9042
export SCYLLADB_KEYSPACE=cloudforge    # optional
export HTTP_ADDR=:8081                 # optional

cd services/cf-accounts
go run .
```

Smoke:

```bash
curl -sS "http://localhost:8081/v1/accounts?limit=20&offset=0"
```

After creating an account, you can log in:

```bash
curl -sS -X POST "http://localhost:8081/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"your-secure-password"}'
```

## Layout

| Path | Role |
|------|------|
| `main.go` | Process entry: Scylla session, repositories, `service.New`, `rest.NewHandler`, `rest.NewRouter`, `http.Server`. |
| `internal/service/` | Business logic ([`AccountsService`](internal/service/api.go)). |
| `internal/repository/` | CQL / Scylla access per aggregate. |
| `internal/rest/` | HTTP: [`handler.go`](internal/rest/handler.go), [`server.go`](internal/rest/server.go), [`errors.go`](internal/rest/errors.go), [`models_transform.go`](internal/rest/models_transform.go), [`generated/`](internal/rest/generated/). |

## Docker

From the **repository root** (requires `go.work` and `go.work.sum`):

```bash
docker build -f services/cf-accounts/Dockerfile .
```

The image runs a static `cf-accounts` binary listening on **`HTTP_ADDR`** (default `:8081`).

## Regenerate API code

```bash
cd services/cf-accounts
go generate ./...
```

This refreshes `internal/rest/generated/server.gen.go` and `libs/clients/cf-accounts/v1/client.gen.go`.
