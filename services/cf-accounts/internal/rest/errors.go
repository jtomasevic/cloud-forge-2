package rest

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	cfmiddleware "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/middleware"

	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/rest/generated"
)

func withRequestID(ctx context.Context, e generated.Error) generated.Error {
	if rid := cfmiddleware.RequestIDFromContext(ctx); rid != "" {
		e.RequestId = &rid
	}
	return e
}

// mapServiceError maps a service/repository error to an HTTP status and generated Error body.
// Raw causes are logged; the body is safe for external clients.
func mapServiceError(ctx context.Context, err error) (int, generated.Error) {
	var cfErr *cferrors.CFError
	if !errors.As(err, &cfErr) {
		slog.ErrorContext(ctx, "unexpected error",
			"error", err,
			"request_id", cfmiddleware.RequestIDFromContext(ctx),
		)
		return http.StatusInternalServerError, withRequestID(ctx, generated.Error{
			Code:    "INTERNAL",
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
	default:
		slog.ErrorContext(ctx, "unmapped CF error",
			"code", cfErr.Code(),
			"error", err,
			"request_id", cfmiddleware.RequestIDFromContext(ctx),
		)
		return http.StatusInternalServerError, withRequestID(ctx, generated.Error{
			Code:    "INTERNAL",
			Message: "internal error",
		})
	}
}
