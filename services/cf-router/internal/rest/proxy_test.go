// Package rest_test covers [rest.RouteTable] and [rest.ProxyHandler] without a live Scylla/Keycloak stack.
package rest_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/rest"
	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/service"
)

type staticRouter struct {
	tc  service.TenantContext
	err error
}

func (s *staticRouter) ValidateAndResolve(ctx context.Context, params service.ValidateParams) (service.TenantContext, error) {
	_ = ctx
	_ = params
	return s.tc, s.err
}

func (s *staticRouter) Ready(ctx context.Context) error { return nil }

func TestRouteTable_Match_accountsPath(t *testing.T) {
	rt := rest.SortedRouteTable(rest.RouteTable{
		{PathPrefix: "/v1/accounts", TargetURL: "http://a", Description: "a"},
		{PathPrefix: "/v1/tenants", TargetURL: "http://b", Description: "b"},
	})
	e, ok := rt.Match("/v1/accounts/123")
	if !ok {
		t.Fatal("expected match")
	}
	if e.TargetURL != "http://a" {
		t.Fatalf("target: %s", e.TargetURL)
	}
}

func TestProxyHandler_injectsTenantHeader_andStripsAPIKey(t *testing.T) {
	var sawTenant string
	var sawAPIKey string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawTenant = r.Header.Get("X-CF-Tenant-ID")
		sawAPIKey = r.Header.Get("X-CF-API-Key")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer up.Close()

	rt := rest.RouteTable{{PathPrefix: "/v1/x", TargetURL: up.URL, Description: "t"}}
	svc := &staticRouter{tc: service.TenantContext{TenantID: "tid-1", AccountID: "aid", NetworkID: "", Region: "us", Status: "active"}}
	h := rest.ProxyHandler(svc, rt, "internal")

	req := httptest.NewRequest(http.MethodGet, "/v1/x/foo", nil)
	req.Header.Set("X-CF-API-Key", "supersecret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	if sawTenant != "tid-1" {
		t.Fatalf("tenant header: %q", sawTenant)
	}
	if sawAPIKey != "" {
		t.Fatalf("api key should be stripped, got %q", sawAPIKey)
	}
}

func TestProxyHandler_noAuth_returns401JSON(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called")
	}))
	defer up.Close()
	rt := rest.RouteTable{{PathPrefix: "/v1/x", TargetURL: up.URL, Description: "t"}}
	svc := &staticRouter{err: service.ErrUnauthenticated}
	h := rest.ProxyHandler(svc, rt, "internal")

	req := httptest.NewRequest(http.MethodGet, "/v1/x/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rr.Code)
	}
	b, _ := io.ReadAll(rr.Body)
	if !strings.Contains(string(b), "UNAUTHORIZED") {
		t.Fatalf("body: %s", b)
	}
}
