package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORSHandlesSwaggerPreflight(t *testing.T) {
	called := false
	handler := WithCORS(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}), CORSConfig{AllowedOrigins: []string{"http://localhost:8090"}})

	req := httptest.NewRequest(http.MethodOptions, "/internal/routes", nil)
	req.Header.Set("Origin", "http://localhost:8090")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "X-CF-Internal-Secret")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Fatal("next handler should not be called for preflight")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: want %d got %d", http.StatusNoContent, rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8090" {
		t.Fatalf("allow origin: %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Fatal("missing Access-Control-Allow-Headers")
	}
}

func TestWithCORSAddsHeadersToSwaggerRequest(t *testing.T) {
	handler := WithCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), CORSConfig{AllowedOrigins: []string{"http://localhost:8090"}})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://localhost:8090")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want %d got %d", http.StatusOK, rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8090" {
		t.Fatalf("allow origin: %q", got)
	}
}
