// Request context helpers for strict OpenAPI handlers (see package [rest] doc in doc.go).
package rest

import (
	"context"
	"net/http"
)

// ctxKey is a private type for context keys (avoid collisions with other packages).
type ctxKey int

// httpRequestCtxKey is the key under which we store *http.Request for strict OpenAPI handlers.
const httpRequestCtxKey ctxKey = 1

// WithHTTPRequest returns a child context that carries r. Used by [attachHTTPRequestMiddleware].
//
// Handlers must use [HTTPRequestFromContext] rather than assuming a global or closure capture,
// because the strict generated stack controls the handler signature.
func WithHTTPRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, httpRequestCtxKey, r)
}

// HTTPRequestFromContext returns the *http.Request previously stored with [WithHTTPRequest], or nil.
func HTTPRequestFromContext(ctx context.Context) *http.Request {
	v, _ := ctx.Value(httpRequestCtxKey).(*http.Request)
	return v
}
