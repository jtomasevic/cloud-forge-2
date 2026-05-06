// Package middleware provides standard HTTP middleware for CloudForge services.
// All middleware is built using only the Go standard library — no third-party
// routers or middleware frameworks are used.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// ContextKey is the unexported type used for all context keys in this package,
// preventing collisions with keys from other packages.
type ContextKey string

// RequestIDKey is the context key under which the request ID is stored.
const RequestIDKey ContextKey = "request_id"

// RequestIDFromContext returns the request ID stored in ctx, or an empty
// string if none is present.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(RequestIDKey).(string)
	return id
}

// ContextWithRequestID returns a copy of ctx with the given request ID stored
// under RequestIDKey.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, RequestIDKey, id)
}

// generateRequestID produces a cryptographically random 16-byte hex string.
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use timestamp-based ID if rand is unavailable (extremely rare).
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

// RequestID is middleware that ensures every request carries a unique request
// ID. If the incoming request already has an X-Request-ID header, that value
// is used. Otherwise a new ID is generated. The ID is stored in the request
// context and echoed back in the X-Request-ID response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = generateRequestID()
		}
		ctx := ContextWithRequestID(r.Context(), id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusResponseWriter wraps http.ResponseWriter to capture the status code
// written by the handler so that Logger can include it in the log entry.
type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusResponseWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// Logger is middleware that logs the method, path, status code, and duration
// of every request at INFO level using log/slog.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", RequestIDFromContext(r.Context()),
		)
	})
}

// Recovery is middleware that recovers from panics in downstream handlers,
// logs the panic value and stack trace at ERROR level, and returns HTTP 500
// to the caller.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.ErrorContext(r.Context(), "panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()),
					"request_id", RequestIDFromContext(r.Context()),
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// MiddlewareFunc is the type for a middleware function that wraps an
// http.Handler and returns an http.Handler.
type MiddlewareFunc func(http.Handler) http.Handler

// Chain composes a handler with one or more middleware functions. Middleware
// is applied in the order given — the first argument is the outermost wrapper:
//
//	Chain(handler, RequestID, Logger, Recovery)
//
// Results in the call order: RequestID → Logger → Recovery → handler.
func Chain(handler http.Handler, middleware ...MiddlewareFunc) http.Handler {
	// Apply in reverse so the first middleware in the list is outermost.
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	return handler
}
