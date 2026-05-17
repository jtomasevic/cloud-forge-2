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
