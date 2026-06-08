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
	accountsPath, ok := spec.Paths["/v1/accounts"].(map[string]interface{})
	if !ok {
		t.Fatal("accounts spec missing /v1/accounts")
	}
	postAccount, ok := accountsPath["post"].(map[string]interface{})
	if !ok {
		t.Fatal("accounts spec missing POST /v1/accounts")
	}
	security, ok := postAccount["security"].([]interface{})
	if !ok {
		t.Fatal("POST /v1/accounts should explicitly override security")
	}
	if len(security) != 0 {
		t.Fatalf("POST /v1/accounts should be public, got security=%#v", security)
	}
	loginPath, ok := spec.Paths["/v1/auth/login"].(map[string]interface{})
	if !ok {
		t.Fatal("accounts spec missing /v1/auth/login")
	}
	postLogin, ok := loginPath["post"].(map[string]interface{})
	if !ok {
		t.Fatal("accounts spec missing POST /v1/auth/login")
	}
	loginSecurity, ok := postLogin["security"].([]interface{})
	if !ok {
		t.Fatal("POST /v1/auth/login should explicitly override security")
	}
	if len(loginSecurity) != 0 {
		t.Fatalf("POST /v1/auth/login should be public, got security=%#v", loginSecurity)
	}
	for _, blocked := range []string{"/internal/v1/resolve", "/v1/networks/{networkId}"} {
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

func TestSwaggerRouterUsesRelativeAPIServerURL(t *testing.T) {
	router := NewSwaggerRouter(SwaggerConfig{APIServerURL: "/"})

	for _, path := range []string{
		"/openapi/cf-router.json",
		"/openapi/cf-accounts.json",
		"/openapi/cf-provisioner.json",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status: want %d got %d body=%s", http.StatusOK, rr.Code, rr.Body.String())
			}
			var spec struct {
				Servers []struct {
					URL string `json:"url"`
				} `json:"servers"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
				t.Fatalf("decode openapi json: %v", err)
			}
			if len(spec.Servers) != 1 || spec.Servers[0].URL != "/" {
				t.Fatalf("servers not rewritten for gateway: %#v", spec.Servers)
			}
		})
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
