// HTTP error mapping to JSON bodies (see package [rest] doc in doc.go).
package rest

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	cfmiddleware "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/middleware"

	apikeysrepo "github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/repository/apikeys"
	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/rest/generated"
	svcerrors "github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/service"
)

// withRequestID copies X-Request-ID from context into the JSON error body when present.
func withRequestID(ctx context.Context, e generated.Error) generated.Error {
	if rid := cfmiddleware.RequestIDFromContext(ctx); rid != "" {
		e.RequestId = &rid
	}
	return e
}

// mapServiceError converts a *cferrors.CFError (or unknown error) into (HTTP status, JSON body).
//
// This is the single mapping table from domain error codes to HTTP semantics for the router's
// JSON error shape ([generated.Error]). Unknown errors are logged and returned as 500 without
// leaking internal details.
func mapServiceError(ctx context.Context, err error) (int, generated.Error) {
	var cfErr *cferrors.CFError
	if !errors.As(err, &cfErr) {
		slog.ErrorContext(ctx, "unexpected error",
			"error", err,
			"request_id", cfmiddleware.RequestIDFromContext(ctx),
		)
		return http.StatusInternalServerError, withRequestID(ctx, generated.Error{
			Code:    string(cferrors.CodeInternal),
			Message: "an unexpected error occurred",
		})
	}

	msg := cfErr.Error()
	body := withRequestID(ctx, generated.Error{
		Code:    string(cfErr.Code()),
		Message: msg,
	})

	switch cfErr.Code() {
	case cferrors.CodeNotFound:
		return http.StatusNotFound, body
	case cferrors.CodeAlreadyExists:
		return http.StatusConflict, body
	case cferrors.CodeInvalidInput:
		return http.StatusBadRequest, body
	case cferrors.CodeUnauthorized:
		return http.StatusUnauthorized, body
	case cferrors.CodeForbidden:
		return http.StatusForbidden, body
	case cferrors.CodeConflict:
		return http.StatusConflict, body
	case cferrors.CodeUnavailable:
		return http.StatusServiceUnavailable, body
	default:
		slog.ErrorContext(ctx, "unmapped CF error",
			"code", cfErr.Code(),
			"error", err,
			"request_id", cfmiddleware.RequestIDFromContext(ctx),
		)
		return http.StatusInternalServerError, withRequestID(ctx, generated.Error{
			Code:    string(cferrors.CodeInternal),
			Message: "internal error",
		})
	}
}

// mapValidateAndResolveError maps credential / resolution failures to HTTP status for
// [Handler.GetInternalTenantContext], which branches explicitly on status.
//
// Note on errors.Is: CloudForge *cferrors.CFError implements Is by **code**, not pointer identity.
// Do not use errors.Is against two different sentinels that share the same Code — it can return true
// for the wrong sentinel. Here we only compare to sentinels with distinct codes, or use explicit
// errors.Is for apikeysrepo.ErrKeyRevoked (Forbidden) vs auth failures (Unauthorized).
func mapValidateAndResolveError(ctx context.Context, err error) int {
	if errors.Is(err, apikeysrepo.ErrKeyRevoked) {
		return http.StatusForbidden
	}
	if errors.Is(err, svcerrors.ErrUnauthenticated) || errors.Is(err, svcerrors.ErrJWTInvalid) || errors.Is(err, svcerrors.ErrTenantResolution) {
		return http.StatusUnauthorized
	}
	if errors.Is(err, svcerrors.ErrAccountsUnreachable) {
		st, _ := mapServiceError(ctx, err)
		return st
	}
	var cfErr *cferrors.CFError
	if errors.As(err, &cfErr) {
		st, _ := mapServiceError(ctx, err)
		return st
	}
	return http.StatusInternalServerError
}
