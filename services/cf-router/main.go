package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	cfaccountsclient "github.com/jtomasevic/cloud-forge-2/libs/clients/cf-accounts/v1"
	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"

	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/repository/apikeys"
	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/rest"
	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	httpAddr := envOr("HTTP_ADDR", ":8083")
	swaggerAddr := envOr("SWAGGER_ADDR", ":8090")
	swaggerAPIServerURL := envOr("SWAGGER_API_BASE_URL", "http://localhost:8083")
	scyllaHosts := envOr("SCYLLADB_HOSTS", "localhost:9042")
	keyspace := envOr("SCYLLADB_KEYSPACE", "cloudforge")
	cfAccountsURL := strings.TrimRight(envOr("CF_ACCOUNTS_URL", "http://localhost:8081"), "/")
	cfProvisionerURL := strings.TrimRight(envOr("CF_PROVISIONER_URL", "http://localhost:8082"), "/")
	internalSecret := envOr("CF_INTERNAL_SECRET", "dev-internal-secret")
	jwksURL := envOr("KEYCLOAK_JWKS_URL", "http://localhost:8084/auth/realms/cloudforge/protocol/openid-connect/certs")
	jwtIssuer := envOr("KEYCLOAK_ISSUER", "")
	corsAllowedOrigins := splitList(envOr("CORS_ALLOWED_ORIGINS", "http://localhost:8090,http://127.0.0.1:8090"))

	session, err := scylladbclient.New(context.Background(), scylladbclient.Config{
		Hosts:    splitHosts(scyllaHosts),
		Keyspace: keyspace,
	})
	if err != nil {
		slog.Error("failed to connect to ScyllaDB", "error", err)
		os.Exit(1)
	}
	defer session.Close()

	cfAccountsClient, err := cfaccountsclient.NewClientWithResponses(cfAccountsURL)
	if err != nil {
		slog.Error("failed to create cf-accounts client", "error", err)
		os.Exit(1)
	}

	routes := rest.DefaultRouteTable(cfAccountsURL, cfProvisionerURL)
	svc := service.New(service.Deps{
		APIKeys:           apikeys.New(session),
		AccountsClient:    cfAccountsClient,
		JWTPublicKeyURL:   jwksURL,
		JWTExpectedIssuer: jwtIssuer,
		InternalSecret:    internalSecret,
	})
	h := rest.NewHandler(svc, routes, internalSecret)
	router := rest.WithCORS(rest.NewRouter(h), rest.CORSConfig{
		AllowedOrigins: corsAllowedOrigins,
	})

	srv := &http.Server{
		Addr:         httpAddr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	apiListener, err := net.Listen("tcp", httpAddr)
	if err != nil {
		slog.Error("failed to listen for cf-router api", "addr", httpAddr, "error", err)
		os.Exit(1)
	}

	errCh := make(chan error, 2)

	if !swaggerServerDisabled(swaggerAddr) {
		swaggerSrv := &http.Server{
			Addr:         swaggerAddr,
			Handler:      rest.NewSwaggerRouter(rest.SwaggerConfig{APIServerURL: swaggerAPIServerURL}),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		swaggerListener, err := net.Listen("tcp", swaggerAddr)
		if err != nil {
			slog.Error("failed to listen for cf-router swagger", "addr", swaggerAddr, "error", err)
			os.Exit(1)
		}
		go serveHTTP("cf-router swagger", swaggerSrv, swaggerListener, errCh)
	}

	go serveHTTP("cf-router api", srv, apiListener, errCh)

	slog.Info("cf-router listening", "addr", httpAddr, "keyspace", keyspace)
	if !swaggerServerDisabled(swaggerAddr) {
		slog.Info("cf-router swagger listening", "addr", swaggerAddr, "apiServerURL", swaggerAPIServerURL)
	}
	if err := <-errCh; err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func serveHTTP(name string, srv *http.Server, listener net.Listener, errCh chan<- error) {
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		errCh <- fmt.Errorf("%s: %w", name, err)
	}
}

func swaggerServerDisabled(addr string) bool {
	switch strings.ToLower(strings.TrimSpace(addr)) {
	case "", "off", "disabled", "false":
		return true
	default:
		return false
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitHosts(s string) []string {
	out := splitList(s)
	if len(out) == 0 {
		return []string{"localhost:9042"}
	}
	return out
}

func splitList(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
