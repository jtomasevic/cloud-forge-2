package main

import (
	"context"
	"log/slog"
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
	scyllaHosts := envOr("SCYLLADB_HOSTS", "localhost:9042")
	keyspace := envOr("SCYLLADB_KEYSPACE", "cloudforge")
	cfAccountsURL := strings.TrimRight(envOr("CF_ACCOUNTS_URL", "http://localhost:8081"), "/")
	cfProvisionerURL := strings.TrimRight(envOr("CF_PROVISIONER_URL", "http://localhost:8082"), "/")
	internalSecret := envOr("CF_INTERNAL_SECRET", "dev-internal-secret")
	jwksURL := envOr("KEYCLOAK_JWKS_URL", "http://localhost:8084/auth/realms/cloudforge/protocol/openid-connect/certs")
	jwtIssuer := envOr("KEYCLOAK_ISSUER", "")

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
	router := rest.NewRouter(h)

	srv := &http.Server{
		Addr:         httpAddr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("cf-router listening", "addr", httpAddr, "keyspace", keyspace)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitHosts(s string) []string {
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
	if len(out) == 0 {
		return []string{"localhost:9042"}
	}
	return out
}
