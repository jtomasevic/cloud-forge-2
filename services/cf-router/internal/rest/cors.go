package rest

import (
	"net/http"
	"strings"
)

// CORSConfig controls the development CORS wrapper used by the public Swagger UI.
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
}

// WithCORS allows the Swagger docs origin to call the CF-Router API port.
func WithCORS(next http.Handler, cfg CORSConfig) http.Handler {
	allowedOrigins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	allowAllOrigins := false
	for _, origin := range cfg.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if origin == "*" {
			allowAllOrigins = true
			continue
		}
		allowedOrigins[origin] = struct{}{}
	}

	allowedMethods := strings.Join(defaultIfEmpty(cfg.AllowedMethods, []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
	}), ", ")
	allowedHeaders := strings.Join(defaultIfEmpty(cfg.AllowedHeaders, []string{
		"Authorization",
		"Content-Type",
		"X-CF-API-Key",
		"X-CF-Internal-Secret",
		"X-Request-ID",
	}), ", ")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAllOrigins || originAllowed(allowedOrigins, origin)) {
			addVary(w, "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
			w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
			w.Header().Set("Access-Control-Max-Age", "600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func defaultIfEmpty(values, defaults []string) []string {
	if len(values) == 0 {
		return defaults
	}
	return values
}

func originAllowed(allowed map[string]struct{}, origin string) bool {
	_, ok := allowed[origin]
	return ok
}

func addVary(w http.ResponseWriter, value string) {
	existing := w.Header().Values("Vary")
	for _, header := range existing {
		for _, part := range strings.Split(header, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	w.Header().Add("Vary", value)
}
