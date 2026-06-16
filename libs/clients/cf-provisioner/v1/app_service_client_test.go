package cfprovisionerclient

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

func mustClientUUID[T any](t *testing.T, raw string) T {
	t.Helper()
	var id T
	if err := json.Unmarshal([]byte(`"`+raw+`"`), &id); err != nil {
		t.Fatalf("parse uuid %q: %v", raw, err)
	}
	return id
}

func TestAppServiceRequestBuildersUseContractPaths(t *testing.T) {
	const server = "http://cf-provisioner.local"
	networkID := mustClientUUID[NetworkId](t, "550e8400-e29b-41d4-a716-446655440000")
	appServiceID := mustClientUUID[AppServiceId](t, "660e8400-e29b-41d4-a716-446655440000")
	subnetID := mustClientUUID[openapi_types.UUID](t, "770e8400-e29b-41d4-a716-446655440000")

	image := "registry.local/cloudforge/invoice-api:dev"
	ports := []AppServicePort{
		{Name: "http", ContainerPort: 8080, Protocol: HTTP},
	}
	createBody := CreateAppServiceJSONRequestBody{
		Name:     "invoice-api",
		SubnetId: subnetID,
		Runtime: AppServiceRuntime{
			ServiceType: Rest,
			Image:       &image,
			Ports:       &ports,
			Resources: AppServiceResources{
				Cpu:    "250m",
				Memory: "256Mi",
			},
		},
	}
	exposureBody := ExposeAppServiceJSONRequestBody{
		Type:    InternetGateway,
		Host:    "invoice-api.local.cloudforge.dev",
		PortRef: "http",
	}

	tests := []struct {
		name   string
		build  func() (*http.Request, error)
		method string
		path   string
	}{
		{
			name: "create",
			build: func() (*http.Request, error) {
				return NewCreateAppServiceRequest(server, networkID, createBody)
			},
			method: http.MethodPost,
			path:   "/v1/networks/550e8400-e29b-41d4-a716-446655440000/app-services",
		},
		{
			name: "list",
			build: func() (*http.Request, error) {
				return NewListAppServicesRequest(server, networkID, nil)
			},
			method: http.MethodGet,
			path:   "/v1/networks/550e8400-e29b-41d4-a716-446655440000/app-services",
		},
		{
			name: "get",
			build: func() (*http.Request, error) {
				return NewGetAppServiceRequest(server, appServiceID)
			},
			method: http.MethodGet,
			path:   "/v1/app-services/660e8400-e29b-41d4-a716-446655440000",
		},
		{
			name: "delete",
			build: func() (*http.Request, error) {
				return NewDeleteAppServiceRequest(server, appServiceID)
			},
			method: http.MethodDelete,
			path:   "/v1/app-services/660e8400-e29b-41d4-a716-446655440000",
		},
		{
			name: "expose",
			build: func() (*http.Request, error) {
				return NewExposeAppServiceRequest(server, appServiceID, exposureBody)
			},
			method: http.MethodPost,
			path:   "/v1/app-services/660e8400-e29b-41d4-a716-446655440000/exposure",
		},
		{
			name: "remove-exposure",
			build: func() (*http.Request, error) {
				return NewRemoveAppServiceExposureRequest(server, appServiceID)
			},
			method: http.MethodDelete,
			path:   "/v1/app-services/660e8400-e29b-41d4-a716-446655440000/exposure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := tt.build()
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if req.Method != tt.method {
				t.Fatalf("method: got %s want %s", req.Method, tt.method)
			}
			if req.URL.Path != tt.path {
				t.Fatalf("path: got %s want %s", req.URL.Path, tt.path)
			}
			if req.URL.Scheme+"://"+req.URL.Host != server {
				t.Fatalf("server: got %s://%s want %s", req.URL.Scheme, req.URL.Host, server)
			}
		})
	}
}

func TestCreateAppServiceRequestBuilderEncodesJSONBody(t *testing.T) {
	networkID := mustClientUUID[NetworkId](t, "550e8400-e29b-41d4-a716-446655440000")
	subnetID := mustClientUUID[openapi_types.UUID](t, "770e8400-e29b-41d4-a716-446655440000")
	image := "registry.local/cloudforge/invoice-api:dev"
	body := CreateAppServiceJSONRequestBody{
		Name:     "invoice-api",
		SubnetId: subnetID,
		Runtime: AppServiceRuntime{
			ServiceType: Rest,
			Image:       &image,
			Resources: AppServiceResources{
				Cpu:    "250m",
				Memory: "256Mi",
			},
		},
	}

	req, err := NewCreateAppServiceRequest("http://cf-provisioner.local", networkID, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type: got %q", got)
	}
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	for _, want := range []string{
		`"name":"invoice-api"`,
		`"subnetId":"770e8400-e29b-41d4-a716-446655440000"`,
		`"serviceType":"rest"`,
		`"image":"registry.local/cloudforge/invoice-api:dev"`,
	} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("payload missing %s: %s", want, payload)
		}
	}
}
