# tools/migrations

CLI that applies **plain CQL migration files** to **ScyllaDB** for the CloudForge `cloudforge` keyspace (accounts, tenants, networks, API keys, provisioning tables, and related DDL owned by CF-Accounts).

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

## How to run

From this directory:

```bash
make help              # list Makefile targets (default `make`)
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
