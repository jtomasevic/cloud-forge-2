package rest

import (
	"context"
	"errors"
	"net/http"
	"testing"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	cfmiddleware "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/middleware"

	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/service"
)

func TestMapServiceError_provisioningFailedIs422(t *testing.T) {
	ctx := cfmiddleware.ContextWithRequestID(context.Background(), "rid-1")
	st, body := mapServiceError(ctx, service.ErrProvisioningFailed)
	if st != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want %d", st, http.StatusUnprocessableEntity)
	}
	if body.Code != "PROVISIONING_FAILED" {
		t.Fatalf("code: got %q", body.Code)
	}
	if body.RequestId == nil || *body.RequestId != "rid-1" {
		t.Fatalf("requestId: got %#v", body.RequestId)
	}
}

func TestMapServiceError_unknownIs500(t *testing.T) {
	ctx := context.Background()
	st, body := mapServiceError(ctx, errors.New("boom"))
	if st != http.StatusInternalServerError {
		t.Fatalf("status: got %d", st)
	}
	if body.Code != "INTERNAL" {
		t.Fatalf("code: got %q", body.Code)
	}
}

func TestMapServiceError_notFound(t *testing.T) {
	ctx := context.Background()
	st, body := mapServiceError(ctx, cferrors.New(cferrors.CodeNotFound, "missing"))
	if st != http.StatusNotFound {
		t.Fatalf("status: got %d", st)
	}
	if body.Code != string(cferrors.CodeNotFound) {
		t.Fatalf("code: got %q", body.Code)
	}
}
