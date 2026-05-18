# `internal/service` — Authentication & tenant resolution

This package is the **business logic** for CF-Router: validate **Bearer JWTs** or **API keys**,
resolve **tenant context** via CF-Accounts, and expose a small façade ([`RouterService`](api.go))
used by [`internal/rest`](../rest/).

Nothing in this package speaks HTTP directly; callers pass [`ValidateParams`](models.go) built from
headers.

## Responsibilities

| Concern | Where |
|--------|--------|
| JWT RS256 verify + JWKS fetch/cache | [`jwt.go`](jwt.go) |
| BLAKE2b-256 API key hashing | [`service.go`](service.go) (`hashAPIKey`) |
| CF-Accounts `GET /internal/v1/resolve` + `X-CF-Internal-Secret` | [`service.go`](service.go) (`resolveTenantViaAccounts`, `internalSecretEditor`) |
| Optional **region** via `GET /v1/networks/{id}` (Bearer replay) | [`service.go`](service.go) (`tenantContextFromResolve`) |
| Readiness probe against CF-Accounts | [`service.go`](service.go) (`Ready`) |
| Typed sentinel errors | [`errors.go`](errors.go) |

## Files

| File | Role |
|------|------|
| [`api.go`](api.go) | [`RouterService`](api.go), [`Deps`](api.go), [`New`](api.go). |
| [`models.go`](models.go) | [`ValidateParams`](models.go), [`TenantContext`](models.go), [`AuthMethod`](models.go). |
| [`errors.go`](errors.go) | `ErrUnauthenticated`, `ErrJWTInvalid`, `ErrTenantResolution`, `ErrAccountsUnreachable`. |
| [`service.go`](service.go) | [`cfRouterService`](service.go) implementation: `ValidateAndResolve`, `Ready`, hashing, CF-Accounts orchestration. |
| [`jwt.go`](jwt.go) | JWKS download, RSA key material, RS256 verification, claim checks. |
| [`doc.go`](doc.go) | Package-level godoc (JWT vs API key, region note). |

## Dependencies (injected)

- **`APIKeys`**: [`internal/repository/apikeys`](../repository/apikeys/) — hash → account.
- **`AccountsClient`**: generated `libs/clients/cf-accounts` — resolve + optional `GetNetwork`.
- **`JWTPublicKeyURL`**: Keycloak (or compatible) JWKS URL.
- **`JWTExpectedIssuer`**: optional `iss` enforcement.
- **`InternalSecret`**: shared mesh secret for CF-Accounts internal endpoints.
- **`HTTPClient`**: optional override for JWKS GETs.

## Related

- HTTP entry + proxy: [`internal/rest`](../rest/)
- OpenAPI / codegen: [`api/cf-router/v1/openapi.yaml`](../../../../api/cf-router/v1/openapi.yaml), [`generate.go`](../generate.go)
