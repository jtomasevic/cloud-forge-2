// Reverse proxy implementation (see package [rest] doc in doc.go for overview).
//
// # Reverse proxy (this file) — mental model
//
// CF-Router sits **in front of** CF-Accounts and CF-Provisioner. Clients talk only to the router.
// For URLs like `/v1/accounts/...`, the router must:
//
//  1. Decide **which upstream service** owns that path (routing table).
//  2. **Authenticate** the caller (JWT and/or API key) and **resolve tenant** (service layer + CF-Accounts).
//     Public signup (`POST /v1/accounts`) and password login (`POST /v1/auth/login`) are the exceptions.
//  3. **Mutate** the outgoing request: add trusted headers (tenant id, internal secret) and **remove**
//     secrets that must never reach upstream (raw API key).
//  4. **Forward** the HTTP method, path, query, and body to the upstream as if the client had called
//     it directly (with extra headers).
//
// We implement (4) with [net/http/httputil.ReverseProxy]. That type is designed for exactly this:
// it copies the incoming request, lets you adjust the outbound URL/headers in a hook called
// **Director**, then performs the upstream HTTP round-trip and streams the response back.
//
// # Why we stash data in [context.Context] for the Director
//
// [httputil.ReverseProxy.Director] receives only `out *http.Request` (the request that will be sent
// upstream). It does not receive our [RouteEntry] or [service.TenantContext] as parameters.
// Those values are computed **before** we call [httputil.ReverseProxy.ServeHTTP], in the outer
// [http.HandlerFunc]. We attach them to the request context under an unexported key (`proxyCtxKey`)
// so the Director can read them back when mutating `out`.
//
// # Request flow (one proxied call)
//
//	Client → CF-Router outer handler ([ProxyHandler] closure):
//	  • Match path prefix → pick upstream base URL (e.g. http://cf-accounts:8081).
//	  • ValidateAndResolve → tenant context (tenant/account/network/region), unless the route is public.
//	  • Clone the original request, enrich context with (route entry + tenant context).
//	  • Delegate to ReverseProxy.ServeHTTP.
//
//	ReverseProxy internally:
//	  • Builds `out` from the cloned request (same path/query/method/body).
//	  • Calls Director(out):
//	      – Set `out.URL.Scheme` and `out.URL.Host` (and `out.Host`) so the HTTP client dials the
//	        correct upstream (the URL path is already the client path, e.g. /v1/accounts/...).
//	      – For authenticated requests, set `X-CF-*` headers from tenant context and set
//	        `X-CF-Internal-Secret` from config.
//	      – For public signup/login, do not inject trusted headers.
//	      – Always delete any client-supplied trusted CloudForge headers and delete `X-CF-API-Key`.
//	  • Performs the upstream request; copies status/headers/body to the client.
//
// If routing fails → 404. If auth/tenant resolution fails → JSON error (no upstream call).
// If upstream is down / connection fails → ErrorHandler returns 502 JSON.
package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"

	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/rest/generated"
	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/service"
)

// RouteEntry is one row in the static routing table: "URLs starting with PathPrefix go to TargetURL".
//
// PathPrefix should look like "/v1/accounts" (leading slash). Match logic tolerates missing leading
// slashes by normalizing internally.
//
// TargetURL is the **origin** of the upstream service (scheme + host + optional port), e.g.
// "http://cf-accounts:8081". The client's request path (/v1/accounts/...) is forwarded as-is; we do
// not strip the prefix — upstreams expect the same public paths they expose behind the router.
type RouteEntry struct {
	PathPrefix  string
	TargetURL   string
	Description string
}

// RouteTable is the full ordered list of route entries used by [RouteTable.Match] and [ProxyHandler].
//
// Order only matters before [SortedRouteTable]: matching is defined as **longest prefix wins**, so
// shorter prefixes should not shadow longer ones; we sort by descending prefix length inside
// [ProxyHandler] to make behavior deterministic even if the caller passes an unsorted slice.
type RouteTable []RouteEntry

// Match returns the route entry whose PathPrefix is the longest prefix of path.
//
// Normalization:
//   - Empty path is treated as "/".
//   - A path without a leading "/" gets one prepended.
//   - A match requires either exact equality (path == prefix) or prefix boundary: next rune after
//     the prefix must be '/' so "/v1/account" does not match prefix "/v1/accounts".
//
// Examples (with a typical table):
//   - "/v1/accounts/550e8400-e29b-41d4-a716-446655440000" → /v1/accounts → CF-Accounts
//   - "/v1/networks/..." → CF-Provisioner
//   - "/health" → no match (handled by native mux routes registered before the catch-all "/").
func (rt RouteTable) Match(path string) (RouteEntry, bool) {
	if path == "" {
		path = "/"
	}
	if path[0] != '/' {
		path = "/" + path
	}
	type scored struct {
		e RouteEntry
		n int
	}
	var best *scored
	for _, e := range rt {
		p := e.PathPrefix
		if p == "" {
			continue
		}
		if p[0] != '/' {
			p = "/" + p
		}
		// Exact match, or prefix match with '/' boundary so we do not split mid-segment.
		if path == p || strings.HasPrefix(path, p) && (len(path) == len(p) || path[len(p)] == '/') {
			if best == nil || len(p) > best.n {
				best = &scored{e: e, n: len(p)}
			}
		}
	}
	if best == nil {
		return RouteEntry{}, false
	}
	return best.e, true
}

// DefaultRouteTable is the static table described in api/cf-router/v1/openapi.yaml (proxy section).
// cfAccountsURL and cfProvisionerURL should be base origins without a path, e.g. http://localhost:8081.
func DefaultRouteTable(cfAccountsURL, cfProvisionerURL string) RouteTable {
	return RouteTable{
		{PathPrefix: "/v1/auth", TargetURL: cfAccountsURL, Description: "Authentication APIs (CF-Accounts)"},
		{PathPrefix: "/v1/accounts", TargetURL: cfAccountsURL, Description: "Account APIs (CF-Accounts)"},
		{PathPrefix: "/v1/tenants", TargetURL: cfAccountsURL, Description: "Tenant APIs (CF-Accounts)"},
		{PathPrefix: "/v1/networks", TargetURL: cfProvisionerURL, Description: "Network APIs (CF-Provisioner)"},
		{PathPrefix: "/v1/gateways", TargetURL: cfProvisionerURL, Description: "Gateway APIs (CF-Provisioner)"},
		{PathPrefix: "/v1/jobs", TargetURL: cfProvisionerURL, Description: "Async job APIs (CF-Provisioner)"},
	}
}

// SortedRouteTable returns a defensive copy of rt sorted by descending PathPrefix length.
// Longest-prefix matching is implemented in [RouteTable.Match]; sorting makes iteration order irrelevant.
func SortedRouteTable(rt RouteTable) RouteTable {
	out := append(RouteTable(nil), rt...)
	sort.Slice(out, func(i, j int) bool {
		return len(out[i].PathPrefix) > len(out[j].PathPrefix)
	})
	return out
}

// proxyCtxKey is the private context key type for values attached to proxied requests.
// Using a dedicated struct type avoids collisions with other packages' context keys.
type proxyCtxKey struct{}

// proxyCtxVal is the payload stored on the request context for the ReverseProxy Director.
//
// entry: which upstream base URL to dial (from the routing table).
// tc:    resolved tenant metadata to inject as X-CF-* headers on the upstream request.
type proxyCtxVal struct {
	entry  RouteEntry
	tc     service.TenantContext
	public bool
}

// ProxyHandler returns an [http.Handler] that implements the "everything except native CF-Router paths"
// branch. It is registered on the root mux as the catch-all pattern "/" (see [NewRouter]).
//
// Security / header contract (Director):
//   - For authenticated requests, set X-CF-Tenant-ID, X-CF-Account-ID, X-CF-Network-ID,
//     X-CF-Region from resolved context (network/region may be empty strings when unknown).
//   - For public signup/login (`POST /v1/accounts`, `POST /v1/auth/login`), forward without tenant
//     context because credentials are being created or verified.
//   - Replace/suppress X-CF-Internal-Secret: delete any client value, then set the configured secret
//     only for authenticated proxied calls.
//   - Strip X-CF-API-Key: the raw secret must not be forwarded; upstream trusts router-injected headers.
//
// Error responses:
//   - No route → 404 ([http.NotFound]).
//   - Auth / tenant resolution failure → JSON from [mapServiceError] (typical 401/403/503).
//   - Upstream failure → 502 JSON from ErrorHandler (opaque to clients; check router logs).
func ProxyHandler(svc service.RouterService, routes RouteTable, internalSecret string) http.Handler {
	routes = SortedRouteTable(routes)

	// ReverseProxy is long-lived; Director runs per outbound request. Do not capture per-request
	// variables in Director by accident—read them from out.Context() instead.
	proxy := &httputil.ReverseProxy{
		Director: func(out *http.Request) {
			v, _ := out.Context().Value(proxyCtxKey{}).(proxyCtxVal)
			target, err := url.Parse(v.entry.TargetURL)
			if err != nil {
				// Misconfiguration: invalid TargetURL. ReverseProxy may still send a broken request;
				// prefer fixing DefaultRouteTable / config at startup in the future.
				return
			}
			// Dial the upstream host while preserving the original URL path & query on `out`.
			out.URL.Scheme = target.Scheme
			out.URL.Host = target.Host
			out.Host = target.Host

			// Never trust client-supplied CloudForge trust headers on the upstream path.
			out.Header.Del("X-CF-Internal-Secret")
			out.Header.Del("X-CF-Tenant-ID")
			out.Header.Del("X-CF-Account-ID")
			out.Header.Del("X-CF-Network-ID")
			out.Header.Del("X-CF-Region")
			// Critical: do not leak the raw API key to CF-Accounts / CF-Provisioner.
			out.Header.Del("X-CF-API-Key")

			if v.public {
				return
			}

			tc := v.tc
			secret := strings.TrimSpace(internalSecret)
			out.Header.Set("X-CF-Tenant-ID", tc.TenantID)
			out.Header.Set("X-CF-Account-ID", tc.AccountID)
			out.Header.Set("X-CF-Network-ID", tc.NetworkID)
			out.Header.Set("X-CF-Region", tc.Region)
			if secret != "" {
				out.Header.Set("X-CF-Internal-Secret", secret)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			ctx := r.Context()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(withRequestID(ctx, generated.Error{
				Code:    "BAD_GATEWAY",
				Message: "upstream request failed",
			}))
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// 1) Pick upstream service by URL path.
		entry, ok := routes.Match(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		public, canonicalPath := publicProxyRequest(r)
		var tc service.TenantContext
		if !public {
			// 2) Authenticate + resolve tenant (Scylla + JWKS + CF-Accounts internal resolve).
			params := service.ValidateParams{
				AuthorizationHeader: r.Header.Get("Authorization"),
				APIKeyHeader:        r.Header.Get("X-CF-API-Key"),
			}
			var err error
			tc, err = svc.ValidateAndResolve(ctx, params)
			if err != nil {
				st, body := mapServiceError(ctx, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(st)
				_ = json.NewEncoder(w).Encode(body)
				return
			}
		}

		// 3) Clone so we do not mutate the original *http.Request seen by middleware/logging.
		//    Attach routing + tenant info for the Director (step 4 inside ReverseProxy).
		pr := r.Clone(ctx)
		if canonicalPath != "" {
			pr.URL.Path = canonicalPath
			pr.URL.RawPath = ""
		}
		pr = pr.WithContext(context.WithValue(pr.Context(), proxyCtxKey{}, proxyCtxVal{entry: entry, tc: tc, public: public}))
		proxy.ServeHTTP(w, pr)
	})
}

func publicProxyRequest(r *http.Request) (bool, string) {
	if r.Method != http.MethodPost {
		return false, ""
	}
	switch r.URL.Path {
	case "/v1/accounts":
		return true, ""
	case "/v1/accounts/":
		return true, "/v1/accounts"
	case "/v1/auth/login":
		return true, ""
	case "/v1/auth/login/":
		return true, "/v1/auth/login"
	default:
		return false, ""
	}
}
