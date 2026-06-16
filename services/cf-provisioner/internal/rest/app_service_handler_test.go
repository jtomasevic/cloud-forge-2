package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/rest/generated"
)

func TestAppServiceRoutesReturnTypedNotImplemented(t *testing.T) {
	const (
		internalSecret = "test-internal-secret"
		requestID      = "rid-app-service"
		networkID      = "550e8400-e29b-41d4-a716-446655440000"
		appServiceID   = "660e8400-e29b-41d4-a716-446655440000"
	)

	createBody := `{
		"name": "invoice-api",
		"subnetId": "770e8400-e29b-41d4-a716-446655440000",
		"runtime": {
			"serviceType": "rest",
			"image": "registry.local/cloudforge/invoice-api:dev",
			"resources": {"cpu": "250m", "memory": "256Mi"},
			"ports": [{"name": "http", "containerPort": 8080, "protocol": "HTTP"}]
		}
	}`
	exposureBody := `{
		"type": "InternetGateway",
		"host": "invoice-api.local.cloudforge.dev",
		"portRef": "http",
		"swagger": {"documentPath": "/openapi.json", "uiPath": "/swagger"}
	}`

	router := NewRouter(NewHandler(nil), internalSecret)
	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		message string
	}{
		{
			name:    "create",
			method:  http.MethodPost,
			path:    "/v1/networks/" + networkID + "/app-services",
			body:    createBody,
			message: "app service creation is not implemented yet",
		},
		{
			name:    "list",
			method:  http.MethodGet,
			path:    "/v1/networks/" + networkID + "/app-services?limit=10&offset=0",
			message: "app service listing is not implemented yet",
		},
		{
			name:    "get",
			method:  http.MethodGet,
			path:    "/v1/app-services/" + appServiceID,
			message: "app service status retrieval is not implemented yet",
		},
		{
			name:    "delete",
			method:  http.MethodDelete,
			path:    "/v1/app-services/" + appServiceID,
			message: "app service deletion is not implemented yet",
		},
		{
			name:    "expose",
			method:  http.MethodPost,
			path:    "/v1/app-services/" + appServiceID + "/exposure",
			body:    exposureBody,
			message: "app service exposure is not implemented yet",
		},
		{
			name:    "remove-exposure",
			method:  http.MethodDelete,
			path:    "/v1/app-services/" + appServiceID + "/exposure",
			message: "app service exposure removal is not implemented yet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("X-CF-Internal-Secret", internalSecret)
			req.Header.Set("X-Request-ID", requestID)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusNotImplemented {
				t.Fatalf("status: want %d got %d body=%s", http.StatusNotImplemented, rr.Code, rr.Body.String())
			}
			if got := rr.Header().Get("X-Request-ID"); got != requestID {
				t.Fatalf("response request id header: got %q want %q", got, requestID)
			}
			var body generated.Error
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if body.Code != "NOT_IMPLEMENTED" {
				t.Fatalf("code: got %q", body.Code)
			}
			if body.Message != tt.message {
				t.Fatalf("message: got %q want %q", body.Message, tt.message)
			}
			if body.RequestId == nil || *body.RequestId != requestID {
				t.Fatalf("requestId: got %#v want %q", body.RequestId, requestID)
			}
		})
	}
}
