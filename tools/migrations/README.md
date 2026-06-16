# tools/migrations

CLI that applies **plain CQL migration files** to **ScyllaDB** for the CloudForge `cloudforge` keyspace (accounts, tenants, networks, API keys, provisioning tables, subnet metadata, CF App Service state, and related control-plane DDL).

## What it does

1. **Bootstraps the keyspace** — Opens a session without a default keyspace, runs `CREATE KEYSPACE IF NOT EXISTS …` (SimpleStrategy, RF=1 for local dev), then closes. This avoids a chicken-and-egg problem: the driver cannot attach to a keyspace that does not exist yet.
2. **Connects** to Scylla with `--keyspace` (default `cloudforge`).
3. **Runs migrations** via `libs/scylladb/pkg/migrate`: reads every `*.cql` file in `--scripts-dir` in **lexicographic filename order**, splits statements on `;`, executes each, and records applied filenames in **`schema_migrations`** so repeats are safe.

Migrations are intended to be **idempotent** (`IF NOT EXISTS` on keyspace and tables).

## Why it exists

- **One place** for schema evolution as checked-in `.cql` files under `scripts/`.
- **Same entrypoint** for local dev, CI, or operators: point at CQL hosts and a scripts directory, run once (or again after failures without double-applying completed files).
- **Thin services** — CF-Accounts and other services do not embed migration orchestration; this tool does.

## When to use it

Use this tool when you need the **database schema created or updated** on any environment where Scylla is reachable on the CQL port, for example:

- Right after **starting Scylla** (Docker Compose, k3d, etc.) on a developer machine.
- In **CI/CD** before integration tests or deploys that assume tables exist.
- After **adding a new ordered `.cql` file** under `scripts/` — run migrations so that cluster picks up the new DDL.

This is an **offline / admin** step, not part of normal HTTP request handling for CF-Accounts or other services.

## Schema Overview

Migration scripts are ordered by filename under [`scripts/`](scripts/). The current state tables are:

| Area | Tables | Purpose |
|------|--------|---------|
| Accounts | `accounts`, `accounts_by_email` | Account identity and email lookup. |
| Tenants | `tenants`, `tenants_by_account`, `tenants_by_slug` | Tenant ownership, slug lookup, and account-scoped listing. |
| Networks | `networks`, `networks_by_tenant`, `cidr_allocations` | Private-network metadata and pod/service CIDR ownership. |
| Credentials | `api_keys`, `api_keys_by_account`, `api_keys_by_hash` | API key lookup without storing raw keys. |
| Provisioning | `provisioning_jobs`, `provisioning_jobs_by_network` | Generic async job polling and network-scoped job history. |
| Subnets | `subnets`, `subnets_by_network`, `subnets_by_network_cidr` | Durable private/public subnet placement metadata. |
| App Services | `app_services`, `app_services_by_network`, `app_service_exposures_by_host`, `app_service_jobs_by_app_service` | Durable workload intent, network listing, public exposure lookup, and app-service lifecycle job correlation. |

The CF App Service MVP stores nested runtime, environment, port, exposure, and Swagger/OpenAPI
fragments in `*_json` text columns. That is intentional for this phase: OpenAPI and service-layer
validation own those nested shapes, while Scylla rows keep tenant/network/subnet/status/exposure
fields queryable without introducing a larger table family before reconciliation behavior is stable.

## How to run

From this directory:

```bash
make help              # list Makefile targets (default `make`)
make test              # unit tests; no ScyllaDB required
make migrate           # needs Scylla on HOSTS (see Makefile variables)
```

Makefile variables (override on the command line):

| Variable      | Default              | Meaning                          |
|---------------|----------------------|----------------------------------|
| `HOSTS`       | `localhost:9042`     | Comma-separated CQL contact points |
| `KEYSPACE`    | `cloudforge`         | Target keyspace                  |
| `SCRIPTS_DIR` | `./scripts`          | Directory of `.cql` files (relative to this module) |

Direct Go invocation:

```bash
go run . -hosts localhost:9042 -keyspace cloudforge -scripts-dir ./scripts
```

From the **repository root**, the root `Makefile` delegates here:

```bash
make migrate
make migrate HOSTS=127.0.0.1:9042 KEYSPACE=cloudforge
```

Build a binary into `bin/migrate` (ignored by repo `.gitignore` for `bin/`):

```bash
make build
```

## Dependencies

- Go **1.26+** (see repo `go.work`).
- Running **ScyllaDB** (or Cassandra-compatible CQL) at the configured `--hosts`.
- Module **`libs/scylladb`** (`pkg/client`, `pkg/migrate`).

## Further reading

- Plan: `docs/plan/07.CFAccountsSchemaAndMigrations.md`
- Migration runner behavior: `libs/scylladb/pkg/migrate/migrate.go`
