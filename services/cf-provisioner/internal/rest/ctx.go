// Request context helpers for strict OpenAPI handlers.
package rest

import (
	"context"
	"net/http"
)

type ctxKey int

const httpRequestCtxKey ctxKey = 1

func WithHTTPRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, httpRequestCtxKey, r)
}

func HTTPRequestFromContext(ctx context.Context) *http.Request {
	v, _ := ctx.Value(httpRequestCtxKey).(*http.Request)
	return v
}
