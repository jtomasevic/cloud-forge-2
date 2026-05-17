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
│  handler.go  server.go      │  Parses HTTP, calls one service method,
│  models.go   errors.go      │  maps result to HTTP response.
│  models_transform.go        │
│  generated/  (oapi-codegen) │  Never touches repositories directly.
└──────────────┬──────────────┘
               │  service-layer models only
               ▼
┌─────────────────────────────┐
│       Service layer         │  internal/service/
│  api.go      service.go     │  All business logic lives here.
│  models.go   password.go    │  Coordinates one or more repositories.
│  errors.go                  │  Owns transaction / session boundaries.
│  models_transform.go        │
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
| `server.go` | `NewRouter(*Handler)` — strict handler + `http.ServeMux` + middleware chain |
| `models.go` | Package docs; optional HTTP-only types not covered by generated code |
| `models_transform.go` | Maps REST ↔ service models |
| `errors.go` | Maps service errors → HTTP status + JSON error body |

See also [`../README.md`](../README.md) (service runbook: signup, login, env vars).

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
| `password.go` | Password policy validation and bcrypt hashing / comparison helpers |
| `models_transform.go` | Maps service ↔ repository models when needed |
| `errors.go` | Service-level typed errors |

Rules:
- Business logic only — no HTTP types, no raw DB types.
- The service coordinates repositories; handlers do not.
- **Passwords**: plaintext passwords exist only on the service boundary (`CreateAccountParams`, `LoginWithPasswordParams`). They are validated, hashed with **bcrypt** (`golang.org/x/crypto/bcrypt`), and persisted as `password_hash` via the accounts repository. The public `Account` model never carries a password or hash.
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
        (email + password; both required in OpenAPI / generated request types)
      calls AccountsService.CreateAccount(ctx, params)
        │
        ▼ service.CreateAccount(ctx, params)
            validates email + password length (see password.go)
            checks email not already registered (accountsRepo.GetByEmail)
            bcrypt-hashes password → password_hash
            inserts account row (accountsRepo.Insert)
            derives default tenant slug from email local part; ensures uniqueness (tenantsRepo.GetBySlug loop)
            inserts default tenant row in "provisioning" (tenantsRepo.Insert)
            returns service.CreateAccountResult{ Account, DefaultTenant }
        │
      maps CreateAccountResult → generated.CreateAccount201JSONResponse
      writes HTTP 201 + JSON body (account + defaultTenant)
```

The handler never calls `accountRepo` directly.
The service never writes to `http.ResponseWriter`.

---

## Example: `LoginWithPassword`

```
POST /v1/auth/login   (OpenAPI: no BearerAuth — security: [])
  │
  ▼ handler.LoginWithPassword(ctx, req)
      maps body → service.LoginWithPasswordParams
      calls AccountsService.LoginWithPassword(ctx, params)
        │
        ▼ service.LoginWithPassword
            loads account by email (accountsRepo.GetByEmail; row includes password_hash)
            rejects if missing hash, inactive account, or bcrypt compare fails
            on mismatch uses ErrInvalidCredentials (same sentinel for unknown email)
            returns service.Account on success
        │
      maps Account → JSON; 401 on ErrInvalidCredentials
```

---

## OpenAPI alignment (service ↔ spec)

| Concern | Spec / generated | Service |
|--------|-------------------|---------|
| Signup body | `CreateAccountRequest` (`email`, `password`) | `CreateAccountParams` |
| Signup response | `CreateAccountResult` (`account`, `defaultTenant`) | `CreateAccountResult` |
| Login body | `LoginRequest` | `LoginWithPasswordParams` |
| Login response | `Account` | `Account` |

Regenerate stubs after changing `api/cf-accounts/v1/openapi.yaml`:

```bash
# from services/cf-accounts/
go generate ./...
```

This re-runs `oapi-codegen` against `api/cf-accounts/v1/openapi.yaml` and
overwrites `internal/rest/generated/server.gen.go` (and the public client under
`libs/clients/cf-accounts/`).
See `internal/rest/generated/README.md` for the full `StrictServerInterface`.
