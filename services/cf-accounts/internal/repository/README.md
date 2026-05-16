# CF-Accounts — repository layer (`internal/repository/`)

This directory is the **data access layer** for CF-Accounts: it is the only place that issues **CQL** against **ScyllaDB** for the domains this service owns (accounts, tenants, networks, API keys / credentials). Higher layers (**service**, then **REST**) depend on **interfaces** defined here, not on driver details.

For how repositories fit into the full **3-layer** flow, see [`../README.md`](../README.md) (diagram, dependency rules, and example request path).

---

## Purpose

- **Encapsulate persistence**: table names, partition keys, denormalized writes, and read paths live behind small, explicit APIs (`Insert`, `GetByID`, `List…`, `UpdateStatus`, `Revoke`, etc.).
- **Isolate ScyllaDB / gocql**: the `libs/scylladb/pkg/client` session is wired in `main` (or tests) and passed to `New(…)`; callers never see raw driver errors on the interface boundary.
- **Align with the schema**: tables and access patterns match the migrations under `tools/migrations/scripts/` (keyspace `cloudforge` by default).

---

## Layout (one package per aggregate)

Each subfolder is a **separate Go package** with **no imports** between subfolders (no `accounts` → `tenants`, etc.). That keeps aggregates independent and avoids accidental shared state or cycles.

| Package | Responsibility |
|---------|------------------|
| [`accounts/`](accounts/) | `accounts` (incl. `password_hash` bcrypt), `accounts_by_email` |
| [`tenants/`](tenants/) | `tenants`, `tenants_by_account`, `tenants_by_slug` |
| [`networks/`](networks/) | `networks`, `networks_by_tenant` |
| [`credentials/`](credentials/) | `api_keys`, `api_keys_by_account`, `api_keys_by_hash` |

### Files per package (convention)

| File | Role |
|------|------|
| `api.go` | **Repository interface** and `New(*scylladbclient.Session) …Repository` returning that interface. |
| `repository.go` | Unexported concrete type implementing the interface; all `Query` / `Exec` / `Iter` / `Scan` logic. |
| `models.go` | **Persistence models** (`*Row` structs) aligned with CQL columns (e.g. `gocql.UUID`, `time.Time`). |
| `errors.go` | Package **sentinel errors** built with `libs/cloudforge-core/pkg/errors` (`CodeNotFound`, `CodeAlreadyExists`, etc.). |
| `commands.go` | **CQL string constants** only; each statement is commented with **bind order** and parameter types. Dynamic `WHERE id IN (…)` uses a documented **prefix** constant plus generated `?` placeholders in code. |
| `repository_test.go` | Non-integration tests (error mapping, small pure helpers) where useful. |

---

## Principles

1. **Interface at the boundary** — The service layer should accept `AccountsRepository`, `TenantsRepository`, etc., not concrete structs, so implementations can be swapped or mocked.
2. **No repository model leakage** — `*Row` types are for this layer only. The service layer uses its own models and **explicit transforms** (see plan Task 8 and `internal/README.md`).
3. **Typed errors only** — Interface methods return errors that are (or wrap) `*cferrors.CFError`. Raw `gocql` errors are mapped to `CodeNotFound`, `CodeAlreadyExists`, `CodeInternal`, etc., never returned naked.
4. **Context propagation** — Every repository method takes `context.Context` and passes it to `WithContext(ctx)` on queries.
5. **Denormalization is explicit** — Inserts/updates that touch multiple tables for the same logical entity are implemented in one repository method with clear ordering; some paths document **eventual consistency** where a denormalized row might lag (see comments in code, e.g. networks `UpdateStatus`).
6. **CQL lives in `commands.go`** — Keeps `repository.go` focused on control flow and binding; SQL/CQL text and bind contracts stay in one place for review and copy into `cqlsh` if needed.

---

## References

- Task spec: [`docs/plan/08.CFAccountsRepositoryLayer.md`](../../../../docs/plan/08.CFAccountsRepositoryLayer.md) (repository contracts and acceptance criteria).
- Schema / migrations: [`docs/plan/07.CFAccountsSchemaAndMigrations.md`](../../../../docs/plan/07.CFAccountsSchemaAndMigrations.md) and [`tools/migrations/README.md`](../../../../tools/migrations/README.md).
- Platform errors: `libs/cloudforge-core/pkg/errors`.
- Scylla client: `libs/scylladb/pkg/client`.

---

## Testing

- **Unit tests** in each package avoid a live cluster; they cover helpers such as UUID validation and error mapping (`gocql.ErrNotFound` → domain not found, revoked key checks for credentials, etc.).
- **Integration tests** (optional, `//go:build integration`) can be added later with a real session or testcontainers; the plan allows either approach.

---

## Adding a new repository package

1. Add a new directory under `internal/repository/<name>/` (still **no** imports from sibling repository packages).
2. Add `api.go`, `models.go`, `errors.go`, `commands.go`, `repository.go` following the table above.
3. Extend Scylla schema via **new ordered `.cql` files** in `tools/migrations/scripts/` if new tables are required.
4. Wire `New(…)` in service composition (`main` or test harness) and keep the REST layer unaware of the new type.
