// Package service_test contains black-box tests for [service.RouterService] and REST proxy wiring.
package service_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	cfaccountsclient "github.com/jtomasevic/cloud-forge-2/libs/clients/cf-accounts/v1"

	apikeysrepo "github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/repository/apikeys"
	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/service"
)

type fakeAPIKeys struct {
	rec apikeysrepo.APIKeyRecord
	err error
}

func (f *fakeAPIKeys) GetByHash(ctx context.Context, keyHash string) (apikeysrepo.APIKeyRecord, error) {
	_ = ctx
	_ = keyHash
	return f.rec, f.err
}

func TestValidateAndResolve_NoCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected call to accounts: %s", r.URL.Path)
	}))
	defer srv.Close()
	cli, err := cfaccountsclient.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(service.Deps{
		APIKeys:         &fakeAPIKeys{},
		AccountsClient:  cli,
		InternalSecret:  "sec",
		JWTPublicKeyURL: srv.URL + "/jwks",
	})
	_, err = svc.ValidateAndResolve(context.Background(), service.ValidateParams{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, service.ErrUnauthenticated) {
		t.Fatalf("expected ErrUnauthenticated, got %v", err)
	}
}

func TestValidateAndResolve_APIKeyRevoked(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	cli, err := cfaccountsclient.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(service.Deps{
		APIKeys:         &fakeAPIKeys{err: apikeysrepo.ErrKeyRevoked},
		AccountsClient:  cli,
		InternalSecret:  "sec",
		JWTPublicKeyURL: srv.URL + "/jwks",
	})
	_, err = svc.ValidateAndResolve(context.Background(), service.ValidateParams{APIKeyHeader: "k"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, apikeysrepo.ErrKeyRevoked) {
		t.Fatalf("expected ErrKeyRevoked, got %v", err)
	}
}
