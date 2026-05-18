// OpenAPI strict handlers for native CF-Router endpoints (see package [rest] doc in doc.go).
package rest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	cfmiddleware "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/middleware"

	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/rest/generated"
	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/service"
)

// Handler implements [generated.StrictServerInterface]: typed OpenAPI entrypoints backed by
// [service.RouterService]. It does **not** implement proxied /v1/* traffic — that lives in
// [ProxyHandler] on the root mux (see [NewRouter]).
type Handler struct {
	svc            service.RouterService
	routes         RouteTable // copy of the routing table for GET /internal/routes introspection
	internalSecret string     // shared secret for operator-only routes on the router itself
}

// NewHandler wires the handler used by the strict generated server.
//
// internalSecret protects [Handler.ListInternalRoutes] (and should match deployment config).
// routes is exposed read-only via ListInternalRoutes for operators.
func NewHandler(svc service.RouterService, routes RouteTable, internalSecret string) *Handler {
	return &Handler{svc: svc, routes: routes, internalSecret: strings.TrimSpace(internalSecret)}
}

// GetHealth is the liveness probe: cheap, no dependencies, always OK if the process runs.
func (h *Handler) GetHealth(ctx context.Context, _ generated.GetHealthRequestObject) (generated.GetHealthResponseObject, error) {
	_ = ctx
	return generated.GetHealth200JSONResponse{
		Status: generated.HealthResponseStatusOk,
	}, nil
}

// GetReady checks dependencies (currently: can we reach CF-Accounts for internal resolve).
// Returns HTTP 200 with degraded body if CF-Accounts is unreachable — Kubernetes can still use this
// as "started" while surfacing dependency status in JSON.
func (h *Handler) GetReady(ctx context.Context, _ generated.GetReadyRequestObject) (generated.GetReadyResponseObject, error) {
	check := generated.ReadyResponseChecksCfAccountsOk
	status := generated.Ok
	if err := h.svc.Ready(ctx); err != nil {
		check = generated.ReadyResponseChecksCfAccountsUnreachable
		status = generated.Degraded
		slog.WarnContext(ctx, "readiness degraded", "error", err)
	}
	var body generated.ReadyResponse
	body.Status = status
	body.Checks.CfAccounts = &check
	return generated.GetReady200JSONResponse(body), nil
}

// ListInternalRoutes returns the configured proxy routing table for operators.
// Secured out-of-band from OpenAPI codegen: requires header X-CF-Internal-Secret == configured secret.
func (h *Handler) ListInternalRoutes(ctx context.Context, _ generated.ListInternalRoutesRequestObject) (generated.ListInternalRoutesResponseObject, error) {
	r := HTTPRequestFromContext(ctx)
	if r == nil || h.internalSecret == "" || r.Header.Get("X-CF-Internal-Secret") != h.internalSecret {
		_, body := mapServiceError(ctx, cferrors.New(cferrors.CodeUnauthorized, "missing or invalid X-CF-Internal-Secret"))
		return generated.ListInternalRoutes401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}, nil
	}
	items := make([]generated.RouteTarget, 0, len(h.routes))
	for _, e := range h.routes {
		items = append(items, generated.RouteTarget{
			PathPrefix:  e.PathPrefix,
			TargetURL:   e.TargetURL,
			Description: e.Description,
		})
	}
	return generated.ListInternalRoutes200JSONResponse(generated.RouteTargetList{Items: items}), nil
}

// GetInternalTenantContext runs the same credential validation as the proxy but returns JSON instead
// of forwarding. Useful for debugging auth without hitting upstreams.
//
// Credentials are read from the original HTTP headers (see [attachHTTPRequestMiddleware]).
func (h *Handler) GetInternalTenantContext(ctx context.Context, _ generated.GetInternalTenantContextRequestObject) (generated.GetInternalTenantContextResponseObject, error) {
	r := HTTPRequestFromContext(ctx)
	if r == nil {
		_, body := mapServiceError(ctx, cferrors.New(cferrors.CodeInternal, "missing HTTP request context"))
		return generated.GetInternalTenantContext500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}, nil
	}
	params := service.ValidateParams{
		AuthorizationHeader: r.Header.Get("Authorization"),
		APIKeyHeader:        r.Header.Get("X-CF-API-Key"),
	}
	tc, err := h.svc.ValidateAndResolve(ctx, params)
	if err != nil {
		st := mapValidateAndResolveError(ctx, err)
		switch st {
		case http.StatusForbidden:
			_, body := mapServiceError(ctx, err)
			return generated.GetInternalTenantContext403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}, nil
		case http.StatusUnauthorized:
			_, body := mapServiceError(ctx, err)
			return generated.GetInternalTenantContext401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}, nil
		case http.StatusServiceUnavailable:
			// OpenAPI only documents 401/403/500; we map dependency failures to 500 for spec fidelity.
			_, body := mapServiceError(ctx, err)
			return generated.GetInternalTenantContext500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}, nil
		default:
			_, body := mapServiceError(ctx, err)
			return generated.GetInternalTenantContext500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}, nil
		}
	}
	out, err := ToTenantContextResponse(tc)
	if err != nil {
		_, body := mapServiceError(ctx, cferrors.Wrap(cferrors.CodeInternal, "mapping tenant context", err))
		return generated.GetInternalTenantContext500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}, nil
	}
	return generated.GetInternalTenantContext200JSONResponse(out), nil
}

// JSONDecodeError is registered on the strict server for malformed JSON bodies (should be rare on
// GET-only router API). Writes a small JSON problem document.
func JSONDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(withRequestID(ctx, generated.Error{
		Code:    string(cferrors.CodeInvalidInput),
		Message: "invalid JSON: " + err.Error(),
	}))
}

// JSONEncodeError runs if encoding a success response to JSON fails (should be extremely rare).
func JSONEncodeError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	slog.ErrorContext(ctx, "response encode error", "error", err, "request_id", cfmiddleware.RequestIDFromContext(ctx))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(withRequestID(ctx, generated.Error{
		Code:    string(cferrors.CodeInternal),
		Message: "failed to encode response",
	}))
}

var _ generated.StrictServerInterface = (*Handler)(nil)
