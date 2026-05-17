# CF-Accounts — REST layer (`internal/rest/`)

Implements the HTTP surface for CF-Accounts using **oapi-codegen** strict server types under [`generated/`](generated/).

| File | Purpose |
|------|---------|
| [`handler.go`](handler.go) | `Handler` implements `generated.StrictServerInterface` — one service call per operation, maps errors via [`errors.go`](errors.go). |
| [`server.go`](server.go) | `NewRouter` composes the generated `ServeMux`, strict handler options (JSON decode/encode errors), and `libs/cloudforge-core/pkg/middleware` (request ID, logging, recovery). |
| [`models_transform.go`](models_transform.go) | Converts generated request/response DTOs ↔ `internal/service` models (explicit field mapping). |
| [`errors.go`](errors.go) | `mapServiceError` maps `*cferrors.CFError` to HTTP status + `generated.Error` (request ID echoed when present). |

Service runbook (signup with password, login, env vars): [`../../README.md`](../../README.md).  
Full internal layout: [`../README.md`](../README.md).
