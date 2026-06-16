package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/rest/generated"
)

const defaultSwaggerAPIServerURL = "http://localhost:8083"

type SwaggerConfig struct {
	APIServerURL string
}

type publicSpecConfig struct {
	ID              string
	SourcePath      string
	AllowedPrefixes []string
	Proxied         bool
}

var publicSpecs = map[string]publicSpecConfig{
	"cf-router": {
		ID: "cf-router",
	},
	"cf-accounts": {
		ID:              "cf-accounts",
		SourcePath:      "api/cf-accounts/v1/openapi.yaml",
		AllowedPrefixes: []string{"/v1/auth", "/v1/accounts", "/v1/tenants"},
		Proxied:         true,
	},
	"cf-provisioner": {
		ID:         "cf-provisioner",
		SourcePath: "api/cf-provisioner/v1/openapi.yaml",
		// CF App Service has a split route shape: collection operations are nested under
		// /v1/networks/{networkId}/app-services, while item and exposure operations live under
		// /v1/app-services/{appServiceId}. Both prefixes must be preserved in the public docs.
		AllowedPrefixes: []string{"/v1/networks", "/v1/app-services", "/v1/jobs"},
		Proxied:         true,
	},
}

// NewSwaggerRouter serves the public, unauthenticated aggregated OpenAPI docs.
func NewSwaggerRouter(configs ...SwaggerConfig) http.Handler {
	cfg := normalizeSwaggerConfig(configs...)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", redirectToSwagger)
	mux.HandleFunc("GET /swagger", redirectToSwagger)
	mux.HandleFunc("GET /swagger/", serveSwaggerPage)
	mux.HandleFunc("GET /openapi.json", serveRouterSwaggerJSON(cfg))
	mux.HandleFunc("GET /swagger.json", serveRouterSwaggerJSON(cfg))
	mux.HandleFunc("GET /openapi/cf-router.json", serveNamedSwaggerJSON("cf-router", cfg))
	mux.HandleFunc("GET /openapi/cf-accounts.json", serveNamedSwaggerJSON("cf-accounts", cfg))
	mux.HandleFunc("GET /openapi/cf-provisioner.json", serveNamedSwaggerJSON("cf-provisioner", cfg))
	return mux
}

func normalizeSwaggerConfig(configs ...SwaggerConfig) SwaggerConfig {
	cfg := SwaggerConfig{APIServerURL: defaultSwaggerAPIServerURL}
	if len(configs) == 0 {
		return cfg
	}
	apiServerURL := strings.TrimSpace(configs[0].APIServerURL)
	if apiServerURL != "" {
		if apiServerURL != "/" {
			apiServerURL = strings.TrimRight(apiServerURL, "/")
		}
		cfg.APIServerURL = apiServerURL
	}
	return cfg
}

func redirectToSwagger(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/swagger" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/swagger/", http.StatusTemporaryRedirect)
}

func serveSwaggerPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(swaggerPageHTML))
}

func serveRouterSwaggerJSON(cfg SwaggerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveSwaggerJSON(w, r, "cf-router", cfg)
	}
}

func serveNamedSwaggerJSON(id string, cfg SwaggerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveSwaggerJSON(w, r, id, cfg)
	}
}

func serveSwaggerJSON(w http.ResponseWriter, _ *http.Request, id string, swaggerCfg SwaggerConfig) {
	spec, err := publicSwaggerJSON(id, swaggerCfg)
	if err != nil {
		http.Error(w, "failed to load OpenAPI spec: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.oai.openapi+json;version=3.0")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(spec)
}

func publicSwaggerJSON(id string, swaggerCfg SwaggerConfig) ([]byte, error) {
	cfg, ok := publicSpecs[id]
	if !ok {
		return nil, fmt.Errorf("unknown public spec %q", id)
	}
	if id == "cf-router" {
		return cfRouterSwaggerJSON(swaggerCfg)
	}

	data, err := readOpenAPIFile(cfg.SourcePath)
	if err != nil {
		return nil, err
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", cfg.SourcePath, err)
	}
	preparePublicSpec(doc, cfg, swaggerCfg)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", cfg.SourcePath, err)
	}
	return out, nil
}

func cfRouterSwaggerJSON(swaggerCfg SwaggerConfig) ([]byte, error) {
	spec, err := generated.GetSpec()
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal cf-router generated spec: %w", err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode cf-router generated spec: %w", err)
	}
	setPublicSpecServer(doc, swaggerCfg)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal cf-router public spec: %w", err)
	}
	return out, nil
}

func readOpenAPIFile(relPath string) ([]byte, error) {
	for _, path := range openAPIPathCandidates(relPath) {
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
	}
	return nil, fmt.Errorf("could not find %s", relPath)
}

func openAPIPathCandidates(relPath string) []string {
	seen := map[string]struct{}{}
	var candidates []string
	add := func(path string) {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		candidates = append(candidates, clean)
	}

	if wd, err := os.Getwd(); err == nil {
		for dir := wd; ; dir = filepath.Dir(dir) {
			add(filepath.Join(dir, relPath))
			if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	add(filepath.Join("/", relPath))
	return candidates
}

func preparePublicSpec(doc map[string]interface{}, cfg publicSpecConfig, swaggerCfg SwaggerConfig) {
	setPublicSpecServer(doc, swaggerCfg)
	filterPaths(doc, cfg.AllowedPrefixes)
	if cfg.Proxied {
		prepareProxiedSecurity(doc)
		appendInfoDescription(doc, "\n\nThis public development spec is served through CF-Router. Use `"+swaggerCfg.APIServerURL+"` as the API server; CF-Router validates Bearer JWTs or `X-CF-API-Key` and injects internal service headers before proxying.")
	}
}

func setPublicSpecServer(doc map[string]interface{}, cfg SwaggerConfig) {
	doc["servers"] = []map[string]string{
		{
			"url":         cfg.APIServerURL,
			"description": "Local development via CF-Router",
		},
	}
}

func filterPaths(doc map[string]interface{}, prefixes []string) {
	if len(prefixes) == 0 {
		return
	}
	paths, ok := doc["paths"].(map[string]interface{})
	if !ok {
		return
	}
	for path := range paths {
		if !pathHasAnyPrefix(path, prefixes) {
			delete(paths, path)
		}
	}
}

func pathHasAnyPrefix(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") || strings.HasPrefix(path, prefix+"{") {
			return true
		}
	}
	return false
}

func prepareProxiedSecurity(doc map[string]interface{}) {
	components := ensureMap(doc, "components")
	securitySchemes := ensureMap(components, "securitySchemes")
	securitySchemes["BearerAuth"] = map[string]interface{}{
		"type":         "http",
		"scheme":       "bearer",
		"bearerFormat": "JWT",
		"description":  "JWT issued by Keycloak and validated by CF-Router before proxying.",
	}
	securitySchemes["ApiKeyAuth"] = map[string]interface{}{
		"type":        "apiKey",
		"in":          "header",
		"name":        "X-CF-API-Key",
		"description": "CloudForge API key validated by CF-Router before proxying.",
	}
	delete(securitySchemes, "InternalSecret")

	doc["security"] = []map[string][]string{
		{"BearerAuth": {}},
		{"ApiKeyAuth": {}},
	}
}

func ensureMap(parent map[string]interface{}, key string) map[string]interface{} {
	if existing, ok := parent[key].(map[string]interface{}); ok {
		return existing
	}
	next := map[string]interface{}{}
	parent[key] = next
	return next
}

func appendInfoDescription(doc map[string]interface{}, suffix string) {
	info, ok := doc["info"].(map[string]interface{})
	if !ok {
		return
	}
	description, _ := info["description"].(string)
	if strings.Contains(description, suffix) {
		return
	}
	info["description"] = strings.TrimRight(description, " \n") + suffix
}

const swaggerPageHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>CloudForge API Swagger</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    body { margin: 0; background: #fff; }
    #fallback { display: none; margin: 24px; font: 16px/1.5 system-ui, sans-serif; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <div id="fallback">
    Swagger UI assets did not load. Raw OpenAPI documents are available at
    <a href="/openapi/cf-router.json">CF-Router</a>,
    <a href="/openapi/cf-accounts.json">CF-Accounts</a>, and
    <a href="/openapi/cf-provisioner.json">CF-Provisioner</a>.
  </div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.addEventListener("load", function () {
      if (!window.SwaggerUIBundle || !window.SwaggerUIStandalonePreset) {
        document.getElementById("fallback").style.display = "block";
        return;
      }
      window.ui = window.SwaggerUIBundle({
        urls: [
          { url: "/openapi/cf-accounts.json", name: "CF-Accounts via CF-Router" },
          { url: "/openapi/cf-provisioner.json", name: "CF-Provisioner via CF-Router" },
          { url: "/openapi/cf-router.json", name: "CF-Router native endpoints" }
        ],
        "urls.primaryName": "CF-Accounts via CF-Router",
        dom_id: "#swagger-ui",
        deepLinking: true,
        persistAuthorization: true,
        tryItOutEnabled: true,
        presets: [
          window.SwaggerUIBundle.presets.apis,
          window.SwaggerUIStandalonePreset
        ],
        plugins: [
          window.SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout"
      });
    });
  </script>
</body>
</html>
`
