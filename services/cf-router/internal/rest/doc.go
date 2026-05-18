// Package rest is CF-Router's HTTP layer: strict OpenAPI handlers for native endpoints and a
// reverse proxy for tenant-scoped platform APIs.
//
// # Two traffic classes
//
// 1) **Native router endpoints** — registered by oapi-codegen on an [http.ServeMux] with specific
// patterns such as "GET /health". These never hit [ProxyHandler].
//
// 2) **Proxied platform traffic** — anything that does not match a native pattern falls through to
// the catch-all "/" handler, which is [ProxyHandler]. That handler authenticates the caller, resolves
// tenant context, mutates headers, and forwards the request to CF-Accounts or CF-Provisioner.
//
// # Reverse proxy (high level)
//
// [ProxyHandler] uses [net/http/httputil.ReverseProxy]. The proxy copies the client's method, path,
// query, headers, and body toward an upstream origin (scheme+host) chosen from a static [RouteTable].
// Before forwarding, we run [service.RouterService.ValidateAndResolve] and attach trusted headers
// (X-CF-Tenant-ID, X-CF-Account-ID, …). We strip X-CF-API-Key so raw API keys never reach upstreams.
//
// For a step-by-step narrative (including why request context carries route + tenant data into the
// proxy Director), see the large comment at the top of proxy.go.
package rest
