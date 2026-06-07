package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSwaggerRouterServesOpenAPIWithoutAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/openapi/cf-router.json", nil)
	rr := httptest.NewRecorder()

	NewSwaggerRouter().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want %d got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	var spec struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Title string `json:"title"`
		} `json:"info"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode openapi json: %v", err)
	}
	if spec.OpenAPI != "3.0.3" || spec.Info.Title != "CF-Router" {
		t.Fatalf("unexpected spec metadata: openapi=%q title=%q", spec.OpenAPI, spec.Info.Title)
	}
}

func TestSwaggerRouterServesAccountsSpecThroughRouter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/openapi/cf-accounts.json", nil)
	rr := httptest.NewRecorder()

	NewSwaggerRouter().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want %d got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	var spec struct {
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
		Paths map[string]interface{} `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode accounts openapi json: %v", err)
	}
	if len(spec.Servers) != 1 || spec.Servers[0].URL != "http://localhost:8083" {
		t.Fatalf("servers not rewritten for router: %#v", spec.Servers)
	}
	if _, ok := spec.Paths["/v1/accounts"]; !ok {
		t.Fatal("accounts spec missing /v1/accounts")
	}
	for _, blocked := range []string{"/v1/auth/login", "/internal/v1/resolve", "/v1/networks/{networkId}"} {
		if _, ok := spec.Paths[blocked]; ok {
			t.Fatalf("accounts spec should not expose unrouted path %s", blocked)
		}
	}
}

func TestSwaggerRouterServesProvisionerSpecThroughRouter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/openapi/cf-provisioner.json", nil)
	rr := httptest.NewRecorder()

	NewSwaggerRouter().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want %d got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	var spec struct {
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
		Components struct {
			SecuritySchemes map[string]interface{} `json:"securitySchemes"`
		} `json:"components"`
		Paths map[string]interface{} `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode provisioner openapi json: %v", err)
	}
	if len(spec.Servers) != 1 || spec.Servers[0].URL != "http://localhost:8083" {
		t.Fatalf("servers not rewritten for router: %#v", spec.Servers)
	}
	if _, ok := spec.Components.SecuritySchemes["InternalSecret"]; ok {
		t.Fatal("provisioner public spec should not expose InternalSecret auth")
	}
	if _, ok := spec.Components.SecuritySchemes["BearerAuth"]; !ok {
		t.Fatal("provisioner public spec missing BearerAuth")
	}
	if _, ok := spec.Paths["/v1/cidr/allocations"]; ok {
		t.Fatal("provisioner public spec should not expose unrouted /v1/cidr/allocations")
	}
	if _, ok := spec.Paths["/v1/networks"]; !ok {
		t.Fatal("provisioner spec missing /v1/networks")
	}
}

func TestSwaggerRouterServesSwaggerPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/swagger/", nil)
	rr := httptest.NewRecorder()

	NewSwaggerRouter().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want %d got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"CloudForge API Swagger", "/openapi/cf-accounts.json", "/openapi/cf-provisioner.json", "SwaggerUIStandalonePreset", "StandaloneLayout"} {
		if !strings.Contains(body, want) {
			t.Fatalf("swagger page missing %q:\n%s", want, body)
		}
	}
}
