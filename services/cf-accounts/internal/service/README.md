# CF-Accounts — service layer (`internal/service/`)

This directory is the **business-logic layer** for CF-Accounts: it implements the **`AccountsService`** API that the REST layer (and CLIs, jobs, or other callers) use. It **coordinates** the repository packages under `internal/repository/` via their **interfaces**—it never issues CQL or imports driver types. It owns validation, cross-aggregate workflows (e.g. signup creates account + default tenant), password hashing, and domain errors surfaced upward.

For the full **3-layer** diagram, dependency rules, and request examples, see [`../README.md`](../README.md).

---

## Purpose

- **Express product behavior** — Operations such as `CreateAccount`, `LoginWithPassword`, tenant listing, network lifecycle, API keys, and internal `ResolveTenantContext` are implemented here as cohesive use cases.
- **Hide persistence details** — Callers work with service models (`Account`, `Tenant`, `CreateAccountResult`, …) and `*cferrors.CFError` values, not `*Row` structs or `gocql` types.
- **Orchestrate multiple repositories** — One service method may call several repositories in a defined order (e.g. insert account, then insert default tenant; or resolve tenant by walking credentials → tenants → networks).
- **Keep secrets off the public model** — Passwords are accepted on input types, hashed (bcrypt) before storage, and never attached to `Account` returned to HTTP clients.

---

## Layout (single package)

Everything in this folder is package **`service`**. There are no sub-packages except generated **test mocks** under [`mocks/`](mocks/).

| Path | Responsibility |
|------|------------------|
| [`api.go`](api.go) | **`AccountsService`** interface and **`Deps`** / **`New(Deps)`** wiring. |
| [`service.go`](service.go) | **`CFAccountsService`** — concrete implementation of all interface methods. |
| [`models.go`](models.go) | Service-level **DTOs** and parameter structs (`Account`, `CreateAccountParams`, `CreateAccountResult`, …). |
| [`models_transform.go`](models_transform.go) | Maps **repository rows ↔ service models** (keeps `*Row` types out of `service.go`). |
| [`errors.go`](errors.go) | Service-level **sentinel errors** (`ErrAccountNotFound`, `ErrInvalidCredentials`, …). |
| [`password.go`](password.go) | Password length checks and **bcrypt** hash/compare helpers used by signup and login. |
| [`generate.go`](generate.go) | `//go:generate` for **mockgen** (`mocks/*.go`). |
| [`service_test.go`](service_test.go) | Unit tests with **gomock** repository fakes. |
| [`mocks/`](mocks/) | Generated **`Mock*Repository`** types — do not edit by hand; regenerate after interface changes. |

### Files (convention)

| File | Role |
|------|------|
| `api.go` | Stable **facade** for upper layers; only place the `AccountsService` interface needs to be read for “what does CF-Accounts do?”. |
| `service.go` | **Use-case implementations**; may grow large—prefer extracting pure helpers (e.g. `password.go`, slug helpers) over scattering logic. |
| `models.go` | **No** persistence tags; IDs as `string` for API friendliness; timestamps as `time.Time`. |
| `models_transform.go` | Single home for “row → domain” and “params → insert row” transforms to avoid import cycles and duplication. |
| `errors.go` | Errors that REST maps to HTTP status; use `errors.Is` against these sentinels in tests and handlers. |

---

## Principles

1. **Depend on repository interfaces only** — `Deps` holds `AccountsRepository`, `TenantsRepository`, etc., never concrete repository structs.
2. **No HTTP in this package** — No `http.Request`, no generated OpenAPI types; the REST layer maps JSON ↔ `CreateAccountParams` and maps `CreateAccountResult` ↔ `generated.CreateAccount201JSONResponse`.
3. **Typed errors** — Return or wrap `*cferrors.CFError` (`libs/cloudforge-core/pkg/errors`); repositories map DB errors before the service sees them, and the service adds domain semantics (`ErrAccountEmailTaken`, `ErrInvalidCredentials`, …).
4. **Context first** — Every exported method takes `context.Context` and passes it through to repositories.
5. **One service method per REST handler (target)** — Each HTTP operation should call exactly one `AccountsService` method to keep tracing and authorization boundaries clear.

---

## References

- Task spec: [`docs/plan/09.CFAccountsServiceLayer.md`](../../../../docs/plan/09.CFAccountsServiceLayer.md).
- Repository layer: [`../repository/README.md`](../repository/README.md).
- Internal layout & OpenAPI notes: [`../README.md`](../README.md).
- HTTP API spec: [`api/cf-accounts/v1/openapi.yaml`](../../../../api/cf-accounts/v1/openapi.yaml).

---

## Testing

- **`service_test.go`** uses `go.uber.org/mock/gomock` with interfaces from `internal/repository/*/api.go`.
- After changing a repository interface, run from `services/cf-accounts`:

  ```bash
  go generate ./internal/service/...
  ```

  to refresh `mocks/*.go`, then `go test ./internal/service/...`.

---

## Extending the service

1. Add or adjust types in `models.go` (and transforms in `models_transform.go` if persistence mapping changes).
2. Add the method to **`AccountsService`** in `api.go` and implement it on **`CFAccountsService`** in `service.go`.
3. Add sentinels to `errors.go` if new failure modes need stable `errors.Is` checks.
4. Regenerate mocks (`go generate`), add/extend `service_test.go`.
5. Update **`api/cf-accounts/v1/openapi.yaml`** and run **`go generate ./...`** from `services/cf-accounts` so REST and `libs/clients/cf-accounts` stay in sync.
