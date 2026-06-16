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
	tc    service.TenantContext
	err   error
	calls int
}

func (s *staticRouter) ValidateAndResolve(ctx context.Context, params service.ValidateParams) (service.TenantContext, error) {
	_ = ctx
	_ = params
	s.calls++
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

func TestDefaultRouteTable_routesAppServicesToProvisioner(t *testing.T) {
	rt := rest.DefaultRouteTable("http://accounts", "http://provisioner")
	tests := []string{
		"/v1/app-services/550e8400-e29b-41d4-a716-446655440000",
		"/v1/app-services/550e8400-e29b-41d4-a716-446655440000/exposure",
		"/v1/networks/550e8400-e29b-41d4-a716-446655440000/app-services",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			e, ok := rt.Match(path)
			if !ok {
				t.Fatal("expected match")
			}
			if e.TargetURL != "http://provisioner" {
				t.Fatalf("target: %s", e.TargetURL)
			}
		})
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

func TestProxyHandler_createAccountBypassesAuthAndStripsTrustedHeaders(t *testing.T) {
	var sawPath string
	var sawAPIKey string
	var sawInternalSecret string
	var sawTenant string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		sawAPIKey = r.Header.Get("X-CF-API-Key")
		sawInternalSecret = r.Header.Get("X-CF-Internal-Secret")
		sawTenant = r.Header.Get("X-CF-Tenant-ID")
		w.WriteHeader(http.StatusCreated)
	}))
	defer up.Close()

	rt := rest.RouteTable{{PathPrefix: "/v1/accounts", TargetURL: up.URL, Description: "accounts"}}
	svc := &staticRouter{err: service.ErrUnauthenticated}
	h := rest.ProxyHandler(svc, rt, "internal")

	req := httptest.NewRequest(http.MethodPost, "/v1/accounts/", strings.NewReader(`{"email":"new@example.com","password":"password123"}`))
	req.Header.Set("X-CF-API-Key", "should-not-forward")
	req.Header.Set("X-CF-Internal-Secret", "client-secret")
	req.Header.Set("X-CF-Tenant-ID", "client-tenant")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	if svc.calls != 0 {
		t.Fatalf("ValidateAndResolve should not be called for public signup, got %d calls", svc.calls)
	}
	if sawPath != "/v1/accounts" {
		t.Fatalf("path: %q", sawPath)
	}
	if sawAPIKey != "" {
		t.Fatalf("api key should be stripped, got %q", sawAPIKey)
	}
	if sawInternalSecret != "" {
		t.Fatalf("internal secret should not be injected for signup, got %q", sawInternalSecret)
	}
	if sawTenant != "" {
		t.Fatalf("tenant header should not be injected for signup, got %q", sawTenant)
	}
}

func TestProxyHandler_loginBypassesAuthAndStripsTrustedHeaders(t *testing.T) {
	var sawPath string
	var sawAPIKey string
	var sawInternalSecret string
	var sawTenant string
	var sawAccount string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		sawAPIKey = r.Header.Get("X-CF-API-Key")
		sawInternalSecret = r.Header.Get("X-CF-Internal-Secret")
		sawTenant = r.Header.Get("X-CF-Tenant-ID")
		sawAccount = r.Header.Get("X-CF-Account-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	rt := rest.RouteTable{{PathPrefix: "/v1/auth", TargetURL: up.URL, Description: "auth"}}
	svc := &staticRouter{err: service.ErrUnauthenticated}
	h := rest.ProxyHandler(svc, rt, "internal")

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login/", strings.NewReader(`{"email":"new@example.com","password":"password123"}`))
	req.Header.Set("X-CF-API-Key", "should-not-forward")
	req.Header.Set("X-CF-Internal-Secret", "client-secret")
	req.Header.Set("X-CF-Tenant-ID", "client-tenant")
	req.Header.Set("X-CF-Account-ID", "client-account")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	if svc.calls != 0 {
		t.Fatalf("ValidateAndResolve should not be called for public login, got %d calls", svc.calls)
	}
	if sawPath != "/v1/auth/login" {
		t.Fatalf("path: %q", sawPath)
	}
	if sawAPIKey != "" {
		t.Fatalf("api key should be stripped, got %q", sawAPIKey)
	}
	if sawInternalSecret != "" {
		t.Fatalf("internal secret should not be injected for login, got %q", sawInternalSecret)
	}
	if sawTenant != "" {
		t.Fatalf("tenant header should not be injected for login, got %q", sawTenant)
	}
	if sawAccount != "" {
		t.Fatalf("account header should not be injected for login, got %q", sawAccount)
	}
}

func TestProxyHandler_listAccountsStillRequiresAuth(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called")
	}))
	defer up.Close()

	rt := rest.RouteTable{{PathPrefix: "/v1/accounts", TargetURL: up.URL, Description: "accounts"}}
	svc := &staticRouter{err: service.ErrUnauthenticated}
	h := rest.ProxyHandler(svc, rt, "internal")

	req := httptest.NewRequest(http.MethodGet, "/v1/accounts", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rr.Code)
	}
	if svc.calls != 1 {
		t.Fatalf("ValidateAndResolve calls: got %d want 1", svc.calls)
	}
}

func TestProxyHandler_otherAuthPathsStillRequireAuth(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called")
	}))
	defer up.Close()

	rt := rest.RouteTable{{PathPrefix: "/v1/auth", TargetURL: up.URL, Description: "auth"}}
	svc := &staticRouter{err: service.ErrUnauthenticated}
	h := rest.ProxyHandler(svc, rt, "internal")

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rr.Code)
	}
	if svc.calls != 1 {
		t.Fatalf("ValidateAndResolve calls: got %d want 1", svc.calls)
	}
}
