# `internal/repository/apikeys` — API key hash lookup (ScyllaDB)

This package is the **data access** slice of API-key authentication for CF-Router. It performs a
single hot-path query: resolve a **BLAKE2b-256 hex digest** of the raw API key to an **account id**
(and key id) using the `api_keys_by_hash` table.

CF-Accounts owns **writes** (create/revoke keys and denormalized rows). CF-Router only **reads** the
hash index so it can call CF-Accounts internal tenant resolution with the owning account.

## Why it exists

- **Performance**: `api_keys_by_hash` is keyed by `key_hash`, so lookup is a single-partition read.
- **Separation of concerns**: Scylla/gocql details stay here; JWT + HTTP + CF-Accounts calls stay in
  [`internal/service`](../../service/) and [`internal/rest`](../../rest/).

## Files

| File | Role |
|------|------|
| [`api.go`](api.go) | [`APIKeyRepository`](api.go) interface and [`New`](api.go) constructor. |
| [`models.go`](models.go) | [`APIKeyRecord`](models.go) — minimal row returned to the service layer. |
| [`repository.go`](repository.go) | CQL query + scan; maps `gocql.ErrNotFound` and revoked timestamps to typed errors. |
| [`errors.go`](errors.go) | [`ErrKeyNotFound`](errors.go), [`ErrKeyRevoked`](errors.go) (`*cferrors.CFError`). |
| [`doc.go`](doc.go) | Package-level godoc. |

## Contract

- **Input**: `keyHash` must be the **lowercase hex** string of BLAKE2b-256 over the raw secret (same
  algorithm CF-Accounts uses when storing keys).
- **Output**: Active keys return `KeyID` and `AccountID` as UUID strings. Revoked keys return
  [`ErrKeyRevoked`](errors.go); missing keys return [`ErrKeyNotFound`](errors.go).

## Related

- Schema: `tools/migrations/scripts/20240101005_create_api_keys.cql`
- Service usage: [`internal/service/service.go`](../../service/service.go) (`ValidateAndResolve` API-key branch)
