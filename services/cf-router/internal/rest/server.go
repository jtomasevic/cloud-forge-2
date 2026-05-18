// HTTP routing: OpenAPI mux registration + middleware chain (see package [rest] doc in doc.go).
package rest

import (
	"context"
	"net/http"

	cfmiddleware "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/middleware"
	strictnethttp "github.com/oapi-codegen/runtime/strictmiddleware/nethttp"

	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/rest/generated"
)

// attachHTTPRequestMiddleware wraps every strict OpenAPI operation so handlers can recover the raw
// [*http.Request] from context.
//
// Why: oapi-codegen's [generated.StrictServerInterface] methods receive (ctx, typedRequestObject) but
// not *http.Request. Some operations (e.g. [Handler.GetInternalTenantContext]) must read headers
// (Authorization, X-CF-API-Key). The middleware stores r on ctx via [WithHTTPRequest]; handlers read
// it back with [HTTPRequestFromContext].
//
// The second parameter is the OpenAPI operationId string; we ignore it because the same middleware
// applies uniformly.
func attachHTTPRequestMiddleware(next strictnethttp.StrictHTTPHandlerFunc, _ string) strictnethttp.StrictHTTPHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request interface{}) (interface{}, error) {
		return next(WithHTTPRequest(ctx, r), w, r, request)
	}
}

// NewRouter builds the root HTTP handler for CF-Router.
//
// Composition (important for understanding dispatch):
//
//  1. [http.NewServeMux] holds all routes.
//  2. [generated.HandlerFromMux] registers **specific** patterns first, e.g. "GET /health",
//     "GET /internal/routes", … These take precedence over broader patterns (Go 1.22+ routing).
//  3. [mux.Handle] with pattern "/" registers a **catch-all** for any path/method not matched above.
//     That handler is [ProxyHandler]: it authenticates, enriches headers, and forwards to CF-Accounts
//     or CF-Provisioner based on path prefix.
//
// Middleware order (outermost last in [cfmiddleware.Chain] because Chain wraps right-to-left):
//
//	Recovery → Logger → RequestID → mux
//
// So the mux sees request IDs and panics are recovered at the edge.
func NewRouter(handler *Handler) http.Handler {
	strict := generated.NewStrictHandlerWithOptions(handler, []strictnethttp.StrictHTTPMiddlewareFunc{attachHTTPRequestMiddleware}, generated.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  JSONDecodeError,
		ResponseErrorHandlerFunc: JSONEncodeError,
	})
	mux := http.NewServeMux()
	generated.HandlerFromMux(strict, mux)
	mux.Handle("/", ProxyHandler(handler.svc, handler.routes, handler.internalSecret))
	return cfmiddleware.Chain(mux,
		cfmiddleware.RequestID,
		cfmiddleware.Logger,
		cfmiddleware.Recovery,
	)
}
