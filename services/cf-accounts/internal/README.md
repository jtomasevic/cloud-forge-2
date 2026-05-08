# CF-Accounts — internal package layout

Everything under `internal/` is private to the `cf-accounts` service.
No other module in the repository can import these packages.

---

## 3-layer architecture

```
HTTP request
     │
     ▼
┌─────────────────────────────┐
│         REST layer          │  internal/rest/
│  handler.go  service.go     │  Parses HTTP, calls one service method,
│  models.go   errors.go      │  maps result to HTTP response.
│  generated/  (oapi-codegen) │  Never touches repositories directly.
└──────────────┬──────────────┘
               │  service-layer models only
               ▼
┌─────────────────────────────┐
│       Service layer         │  internal/service/
│  api.go      service.go     │  All business logic lives here.
│  models.go   errors.go      │  Coordinates one or more repositories.
│  models_transform.go        │  Owns transaction / session boundaries.
└──────┬──────────────┬───────┘
       │              │  repository-layer models only
       ▼              ▼
┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐
│ repository │  │ repository │  │ repository │  │ repository │
│ /accounts  │  │ /tenants   │  │ /networks  │  │/credentials│
└────────────┘  └────────────┘  └────────────┘  └────────────┘
       internal/repository/  — one subfolder per aggregate
```

---

## Layer rules

### REST layer (`internal/rest/`)

| File | Responsibility |
|---|---|
| `generated/server.gen.go` | oapi-codegen output — **never edit by hand** |
| `handler.go` | Implements `generated.StrictServerInterface` |
| `service.go` | `NewRouter()`, middleware chain (`net/http` only) |
| `models.go` | HTTP-only models not covered by generated types |
| `models_transform.go` | Maps REST ↔ service models |
| `errors.go` | Maps service errors → HTTP status + error body |

Rules:
- Each handler method calls **exactly one service method**.
- REST models must never cross down to the service layer.
- Use `models_transform.go` for every crossing — no inline conversions.

### Service layer (`internal/service/`)

| File | Responsibility |
|---|---|
| `api.go` | Public interface + `New(Deps)` constructor |
| `service.go` | Concrete implementation (`CFAccountsService`) |
| `models.go` | Service-level models |
| `models_transform.go` | Maps service ↔ repository models when needed |
| `errors.go` | Service-level typed errors |

Rules:
- Business logic only — no HTTP types, no raw DB types.
- The service coordinates repositories; handlers do not.
- The concrete struct is named `CF<InterfaceName>` (e.g. `CFAccountsService`).
- `New()` returns the interface type, never the concrete type.

### Repository layer (`internal/repository/<aggregate>/`)

| File | Responsibility |
|---|---|
| `api.go` | Repository interface + `New(Deps)` constructor |
| `repository.go` | Concrete ScyllaDB implementation |
| `models.go` | DB-level models (mapped from/to ScyllaDB rows) |
| `errors.go` | Repository-level typed errors |

Rules:
- One subfolder per aggregate (accounts, tenants, networks, credentials).
- Raw DB SDK types (`*gocql.Session` etc.) never leave the repository package.
- Services depend on the repository **interface**, not the concrete type.

---

## Dependency flow

```
main.go
  │  creates concrete DB session (libs/scylladb)
  │  creates concrete repositories
  │  injects repositories into service constructor
  │  injects service into handler constructor
  │  registers handler on router
  │
  └─► AccountRepo    ─┐
      TenantRepo      ├─► CFAccountsService ──► Handler ──► net/http mux
      NetworkRepo     │
      CredentialsRepo ─┘
```

`main.go` is the **only place** that knows the full object graph.
Every layer depends only on interfaces, so each can be tested in isolation
with a mock or in-memory fake.

---

## Example: `CreateAccount`

```
POST /v1/accounts
  │
  ▼ handler.CreateAccount(ctx, req)
      validates + maps CreateAccountJSONBody → service.CreateAccountParams
      calls AccountService.CreateAccount(ctx, params)
        │
        ▼ service.CreateAccount(ctx, params)
            checks email uniqueness via accountRepo.ExistsByEmail(ctx, email)
            creates Account model
            calls accountRepo.Insert(ctx, account)
            returns service.Account{}
        │
      maps service.Account → generated.Account (REST model)
      writes HTTP 201 + JSON body
```

The handler never calls `accountRepo` directly.
The service never writes to `http.ResponseWriter`.

---

## Regenerating server stubs

```bash
# from services/cf-accounts/
go generate ./...
```

This re-runs `oapi-codegen` against `api/cf-accounts/v1/openapi.yaml` and
overwrites `internal/rest/generated/server.gen.go`.
See `internal/rest/generated/README.md` for the full `StrictServerInterface`.
