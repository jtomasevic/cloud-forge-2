# libs/scylladb

CloudForge ScyllaDB library — a thin, opinionated wrapper around the
[gocql](https://github.com/gocql/gocql) driver plus a deterministic schema
migration runner.

---

## Packages

| Package | Purpose |
|---------|---------|
| `pkg/client` | Session creation, query helpers, sentinel errors |
| `pkg/migrate` | CQL file migration runner |

---

## pkg/client

### Session creation

```go
import (
    "context"

    scylladb "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

sess, err := scylladb.New(context.Background(), scylladb.Config{
    Hosts:    []string{"localhost:9042"},
    Keyspace: "cloudforge",
    Username: "cassandra",
    Password: "cassandra",
})
if err != nil {
    // err wraps client.ErrConnectionFailed — use errors.Is for detection
    return err
}
defer sess.Close()
```

**Config defaults**

| Field | Default |
|-------|---------|
| `Timeout` | `10s` |
| `NumConns` | `2` |
| `Consistency` | `LOCAL_QUORUM` |

### Error handling

```go
import (
    "errors"

    scylladb "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
    cferrors  "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

if errors.Is(err, scylladb.ErrConnectionFailed) {
    // ScyllaDB is unreachable — propagate as CFError with CodeUnavailable
}
if errors.Is(err, cferrors.ErrUnavailable) {
    // Alternative: check the CF-level code
}
```

### Session API

```go
// DDL or DML that returns no rows
sess.ExecCQL(ctx, "INSERT INTO ...")

// Full-flexibility raw query
sess.Query("SELECT id FROM tenants WHERE id = ?", id).WithContext(ctx).Scan(&result)

// Convenience: read a single TEXT column into a string slice
filenames, err := sess.SelectStrings(ctx, "SELECT filename FROM schema_migrations")
```

---

## pkg/migrate

Applies `.cql` migration files in lexicographic (filename) order. Uses a
`schema_migrations` table for idempotency — a file is skipped if its name is
already recorded there.

### Usage

```go
import (
    "context"

    scylladb "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
    "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/migrate"
)

sess, err := scylladb.New(ctx, cfg)
// ... error handling

err = migrate.RunMigrations(ctx, migrate.MigrationConfig{
    Session:    sess,
    ScriptsDir: "migrations/",
})
```

### File conventions

- Files **must** end in `.cql`.
- Files are sorted lexicographically, so prefix with a zero-padded number:
  `001_create_tenants.cql`, `002_add_network_column.cql`, …
- Multiple statements per file are supported — separate them with `;`.
- Non-`.cql` files and directories in `ScriptsDir` are silently ignored.

### Migration table

```cql
CREATE TABLE IF NOT EXISTS schema_migrations (
    filename   TEXT PRIMARY KEY,
    applied_at TIMESTAMP
);
```

This table is created automatically on the first run.

---

## Testing

### Unit tests (no live database required)

- `pkg/client` — exercises `New()` error paths using an unreachable host
  (`127.0.0.1:1`) and tests the internal `scanStringIter` helper via a fake
  iterator.
- `pkg/migrate` — uses a `fakeQuerier` (in-memory stub) for all database
  interactions.

```bash
make test            # run unit tests (-race)
make coverage        # unit tests + per-function coverage report
```

### Integration tests

Integration tests use [testcontainers-go](https://testcontainers.com/guides/getting-started-with-testcontainers-for-go/)
to spin up a real `scylladb/scylla:6.2` container automatically.
Docker must be running on the machine.

```bash
make integration-test          # start container + run all integration tests
```

The container is started **once per package** (`TestMain`) and terminated when
all tests in that package finish. Startup time is typically 30–60 s
(`--developer-mode=1 --smp=1`).

#### Using an existing ScyllaDB instance

Set `SCYLLADB_HOST` to bypass container creation (useful in CI with a
dedicated service, or with the Option-A Makefile approach):

```bash
SCYLLADB_HOST=localhost:9042 make integration-test
```

#### What the integration tests cover

| Package | Tests |
|---------|-------|
| `pkg/client` | `New()` connects; `ExecCQL` DDL/DML; `SelectStrings` returns rows; raw `Query().Scan` |
| `pkg/migrate` | applies both fixture files; idempotent second run; verifies tables exist |

#### Test data

Migration fixture files live in `pkg/migrate/testdata/migrations/`:

| File | What it creates |
|------|----------------|
| `001_create_items.cql` | `cf_migrate_test.items` table |
| `002_add_tags_table.cql` | `cf_migrate_test.tags` table + secondary index |

These files are applied against a keyspace (`cf_migrate_test`) pre-created by
`TestMain` and dropped after the test run.

---

## Development commands

```bash
make build              # compile all packages
make test               # run unit tests with -race
make coverage           # print per-function and total coverage
make coverage-html      # open HTML coverage in browser
make integration-test   # run integration tests (Docker required)
make lint               # go vet (+ golangci-lint if available)
make tidy               # go mod tidy
make clean              # remove coverage artefacts
make help               # list all targets
```

Current unit-test coverage: **≥ 91 %** (100 % on `pkg/migrate`).
