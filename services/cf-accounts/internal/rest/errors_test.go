package rest

import (
	"context"
	"errors"
	"net/http"
	"testing"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	cfmiddleware "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/middleware"

	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/rest/generated"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/service"
)

func TestMapServiceError_NotFound(t *testing.T) {
	ctx := context.Background()
	st, body := mapServiceError(ctx, service.ErrAccountNotFound)
	if st != http.StatusNotFound {
		t.Fatalf("status: want %d got %d", http.StatusNotFound, st)
	}
	if body.Code != string(cferrors.CodeNotFound) {
		t.Fatalf("code: got %q", body.Code)
	}
}

func TestMapServiceError_AlreadyExists(t *testing.T) {
	ctx := context.Background()
	st, body := mapServiceError(ctx, service.ErrAccountEmailTaken)
	if st != http.StatusConflict {
		t.Fatalf("status: want %d got %d", http.StatusConflict, st)
	}
	if body.Code != string(cferrors.CodeAlreadyExists) {
		t.Fatalf("code: got %q", body.Code)
	}
}

func TestMapServiceError_Unauthorized(t *testing.T) {
	ctx := context.Background()
	st, body := mapServiceError(ctx, service.ErrInvalidCredentials)
	if st != http.StatusUnauthorized {
		t.Fatalf("status: want %d got %d", http.StatusUnauthorized, st)
	}
	if body.Code != string(cferrors.CodeUnauthorized) {
		t.Fatalf("code: got %q", body.Code)
	}
}

func TestMapServiceError_NonCFError(t *testing.T) {
	ctx := context.Background()
	st, body := mapServiceError(ctx, errors.New("something exploded"))
	if st != http.StatusInternalServerError {
		t.Fatalf("status: want %d got %d", http.StatusInternalServerError, st)
	}
	if body.Code != "INTERNAL" {
		t.Fatalf("code: want INTERNAL got %q", body.Code)
	}
	if body.Message != "an unexpected error occurred" {
		t.Fatalf("message should be generic, got %q", body.Message)
	}
}

func TestWithRequestID(t *testing.T) {
	ctx := cfmiddleware.ContextWithRequestID(context.Background(), "rid-123")
	e := withRequestID(ctx, generated.Error{Code: "NOT_FOUND", Message: "x"})
	if e.RequestId == nil || *e.RequestId != "rid-123" {
		t.Fatalf("request id not set: %+v", e)
	}
}
