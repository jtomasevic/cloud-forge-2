# `internal/rest` — CF-Router HTTP surface

This package is the **HTTP boundary** for CF-Router. It combines:

1. **Native OpenAPI routes** — `/health`, `/ready`, `/internal/routes`, `/internal/tenant-context` —
   implemented as a **strict** oapi-codegen server ([`generated/`](generated/)).
2. **Reverse proxy** — all other paths (e.g. `/v1/accounts/...`) hit [`ProxyHandler`](proxy.go),
   which authenticates, injects tenant headers, and forwards to CF-Accounts or CF-Provisioner.

Orchestration and auth logic live in [`internal/service`](../service/); Scylla reads live in
[`internal/repository/apikeys`](../repository/apikeys/).

## Why two mechanisms on one mux

Go 1.22+ `http.ServeMux` matches **more specific** patterns first. We register explicit routes via
[`generated.HandlerFromMux`](generated/server.gen.go), then register **`/`** as a catch-all for
everything else ([`NewRouter`](server.go)). So `/health` never reaches the proxy; `/v1/...` does.

## Reverse proxy (mental model)

[`ProxyHandler`](proxy.go):

1. **Match** the URL path against a static [`RouteTable`](proxy.go) (longest prefix wins).
2. **Validate** credentials and **resolve** tenant context via [`service.RouterService`](../service/api.go).
3. **Clone** the request, attach route + tenant data on **context** (the `httputil.ReverseProxy`
   **Director** only receives the outbound `*http.Request`).
4. **Director** sets upstream `Scheme`/`Host`, writes `X-CF-*` headers, sets mesh
   `X-CF-Internal-Secret`, and **deletes** `X-CF-API-Key` so secrets never reach upstreams.
5. **Stream** the upstream response back to the client.

Full narrative: top-of-file comment in [`proxy.go`](proxy.go).

## Files

| File | Role |
|------|------|
| [`server.go`](server.go) | [`NewRouter`](server.go): mux + strict handler + `ProxyHandler` + middleware chain. |
| [`handler.go`](handler.go) | Implements `generated.StrictServerInterface` (health, ready, internal routes, debug tenant context). |
| [`proxy.go`](proxy.go) | [`RouteTable`](proxy.go), [`Match`](proxy.go), [`DefaultRouteTable`](proxy.go), [`ProxyHandler`](proxy.go). |
| [`errors.go`](errors.go) | `mapServiceError`, `mapValidateAndResolveError`, `withRequestID`. |
| [`models_transform.go`](models_transform.go) | `ToTenantContextResponse` — service DTO → OpenAPI JSON. |
| [`ctx.go`](ctx.go) | Stores `*http.Request` on context for strict handlers (see middleware below). |
| [`doc.go`](doc.go) | Package-level godoc. |
| [`generated/`](generated/) | oapi-codegen output — do not hand-edit. |

## Strict server + raw headers

oapi-codegen’s strict interface does not pass `*http.Request` into handler methods. We attach the
request with [`attachHTTPRequestMiddleware`](server.go) + [`WithHTTPRequest`](ctx.go) so
[`Handler.GetInternalTenantContext`](handler.go) can read `Authorization` and `X-CF-API-Key`.

## Regenerate after OpenAPI changes

```bash
cd services/cf-router && go generate ./...
```

Also regenerates the optional public client at `libs/clients/cf-router/v1/client.gen.go`.

## Related

- Spec: [`api/cf-router/v1/openapi.yaml`](../../../../api/cf-router/v1/openapi.yaml)
- Service: [`internal/service/README.md`](../service/README.md)
- API keys repo: [`internal/repository/apikeys/README.md`](../repository/apikeys/README.md)
