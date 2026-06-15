package rest

import (
	"context"
	"net/http"
	"strings"

	cfmiddleware "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/middleware"

	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/rest/generated"
)

// InternalSecretAuth enforces X-CF-Internal-Secret for all routes (OpenAPI security).
func InternalSecretAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.TrimSpace(secret) == "" || r.Header.Get("X-CF-Internal-Secret") != secret {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// internalSecretMW adapts [InternalSecretAuth] to [cfmiddleware.MiddlewareFunc] so it can be chained
// after RequestID, Logger, and Recovery (secret runs just before the OpenAPI mux).
func internalSecretMW(secret string) cfmiddleware.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return InternalSecretAuth(secret)(next)
	}
}

func attachHTTPRequestMiddleware(next generated.StrictHandlerFunc, _ string) generated.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		return next(WithHTTPRequest(ctx, r), w, r, request)
	}
}

// NewRouter returns the root HTTP handler: OpenAPI routes, strict typing, then the standard
// middleware chain and internal-secret auth immediately before the mux.
func NewRouter(handler *Handler, internalSecret string) http.Handler {
	strict := generated.NewStrictHandlerWithOptions(handler, []generated.StrictMiddlewareFunc{attachHTTPRequestMiddleware}, generated.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  JSONDecodeError,
		ResponseErrorHandlerFunc: JSONEncodeError,
	})
	mux := http.NewServeMux()
	generated.HandlerFromMux(strict, mux)
	return cfmiddleware.Chain(mux,
		cfmiddleware.RequestID,
		cfmiddleware.Logger,
		cfmiddleware.Recovery,
		internalSecretMW(internalSecret),
	)
}
