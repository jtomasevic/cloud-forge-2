package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/accounts"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/credentials"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/identity"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/networks"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/tenants"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/rest"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	hosts := envOr("SCYLLADB_HOSTS", "localhost:9042")
	keyspace := envOr("SCYLLADB_KEYSPACE", "cloudforge")
	addr := envOr("HTTP_ADDR", ":8081")
	identityProvider, err := identity.NewKeycloakProvider(identity.KeycloakConfig{
		BaseURL:     envOr("KEYCLOAK_ADMIN_URL", "http://localhost:8084/auth"),
		Realm:       envOr("KEYCLOAK_REALM", "cloudforge"),
		AdminRealm:  envOr("KEYCLOAK_ADMIN_REALM", "master"),
		AdminClient: envOr("KEYCLOAK_ADMIN_CLIENT_ID", "admin-cli"),
		AdminSecret: os.Getenv("KEYCLOAK_ADMIN_CLIENT_SECRET"),
		AdminUser:   envOr("KEYCLOAK_ADMIN_USERNAME", "admin"),
		AdminPass:   envOr("KEYCLOAK_ADMIN_PASSWORD", "admin"),
		LoginClient: envOr("KEYCLOAK_LOGIN_CLIENT_ID", "cf-console"),
		LoginSecret: os.Getenv("KEYCLOAK_LOGIN_CLIENT_SECRET"),
	})
	if err != nil {
		slog.Error("failed to initialize identity provider", "error", err)
		os.Exit(1)
	}

	session, err := scylladbclient.New(context.Background(), scylladbclient.Config{
		Hosts:    splitHosts(hosts),
		Keyspace: keyspace,
	})
	if err != nil {
		slog.Error("failed to connect to ScyllaDB", "error", err)
		os.Exit(1)
	}
	defer session.Close()

	svc := service.New(service.Deps{
		Accounts:    accounts.New(session),
		Tenants:     tenants.New(session),
		Networks:    networks.New(session),
		Credentials: credentials.New(session),
		Identity:    identityProvider,
	})

	h := rest.NewHandler(svc)
	router := rest.NewRouter(h)

	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("cf-accounts listening", "addr", addr, "keyspace", keyspace)
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
