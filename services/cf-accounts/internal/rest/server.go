package rest

import (
	"net/http"

	cfmiddleware "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/middleware"

	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/rest/generated"
)

// NewRouter returns the root HTTP handler for CF-Accounts: OpenAPI routes on a
// ServeMux, strict handlers for request/response typing, then RequestID, Logger, and Recovery.
func NewRouter(handler *Handler) http.Handler {
	strict := generated.NewStrictHandlerWithOptions(handler, nil, generated.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  JSONDecodeError,
		ResponseErrorHandlerFunc: JSONEncodeError,
	})
	mux := http.NewServeMux()
	generated.HandlerFromMux(strict, mux)
	return cfmiddleware.Chain(mux,
		cfmiddleware.RequestID,
		cfmiddleware.Logger,
		cfmiddleware.Recovery,
	)
}
