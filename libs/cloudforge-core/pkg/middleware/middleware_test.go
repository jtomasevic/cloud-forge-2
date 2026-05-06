package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/middleware"
)

// ---------- RequestID ----------

func TestRequestID_GeneratesID(t *testing.T) {
	var capturedID string
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = middleware.RequestIDFromContext(r.Context())
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if capturedID == "" {
		t.Error("expected a request ID to be injected into context")
	}
	if rec.Header().Get("X-Request-ID") != capturedID {
		t.Errorf("response header X-Request-ID %q != context ID %q",
			rec.Header().Get("X-Request-ID"), capturedID)
	}
}

func TestRequestID_UsesExistingHeader(t *testing.T) {
	const existingID = "existing-id-123"
	var capturedID string
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = middleware.RequestIDFromContext(r.Context())
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", existingID)
	handler.ServeHTTP(rec, req)

	if capturedID != existingID {
		t.Errorf("expected existing ID %q to be preserved, got %q", existingID, capturedID)
	}
	if rec.Header().Get("X-Request-ID") != existingID {
		t.Errorf("expected response header to echo existing ID")
	}
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	ids := make(map[string]bool)
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids[middleware.RequestIDFromContext(r.Context())] = true
	}))

	for i := 0; i < 50; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(rec, req)
	}

	if len(ids) != 50 {
		t.Errorf("expected 50 unique IDs, got %d", len(ids))
	}
}

// ---------- Logger ----------

func TestLogger_DoesNotPanic(t *testing.T) {
	handler := middleware.Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)

	// Should not panic.
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestLogger_CapturesNonOKStatus(t *testing.T) {
	handler := middleware.Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ---------- Recovery ----------

func TestRecovery_Returns500OnPanic(t *testing.T) {
	handler := middleware.Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 after panic, got %d", rec.Code)
	}
}

func TestRecovery_DoesNotAffectNormalHandler(t *testing.T) {
	handler := middleware.Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ok", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}
}

// ---------- Chain ----------

func TestChain_AppliesMiddlewareInOrder(t *testing.T) {
	order := []string{}

	makeMiddleware := func(name string) middleware.MiddlewareFunc {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name+":before")
				next.ServeHTTP(w, r)
				order = append(order, name+":after")
			})
		}
	}

	handler := middleware.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
		}),
		makeMiddleware("A"),
		makeMiddleware("B"),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	// A is outermost: A:before → B:before → handler → B:after → A:after
	expected := []string{"A:before", "B:before", "handler", "B:after", "A:after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("step %d: expected %q, got %q", i, v, order[i])
		}
	}
}

func TestChain_WorksWithNoMiddleware(t *testing.T) {
	handler := middleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("expected 418, got %d", rec.Code)
	}
}

// ---------- Context helpers ----------

func TestContextWithRequestID_RoundTrip(t *testing.T) {
	ctx := middleware.ContextWithRequestID(context.Background(), "test-id")
	got := middleware.RequestIDFromContext(ctx)
	if got != "test-id" {
		t.Errorf("expected %q, got %q", "test-id", got)
	}
}

func TestRequestIDFromContext_EmptyWhenMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if id := middleware.RequestIDFromContext(req.Context()); id != "" {
		t.Errorf("expected empty string for missing ID, got %q", id)
	}
}
