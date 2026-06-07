package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	openbaoClient "github.com/jtomasevic/cloud-forge-2/libs/openbao/pkg/client"
	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/cidr"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/cilium"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/gateway"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/jobs"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/kubeconfig"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/vcluster"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/rest"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	addr := envOr("HTTP_ADDR", ":8082")
	scyllaHosts := envOr("SCYLLADB_HOSTS", "localhost:9042")
	keyspace := envOr("SCYLLADB_KEYSPACE", "cloudforge")
	openbaoAddr := envOr("OPENBAO_ADDR", "http://localhost:8200")
	openbaoToken := envOr("OPENBAO_TOKEN", "dev-root-token")
	internalSecret := envOr("CF_INTERNAL_SECRET", "dev-internal-secret")
	kubeConfigPath := envOr("HOST_KUBECONFIG", "")

	session, err := scylladbclient.New(context.Background(), scylladbclient.Config{
		Hosts:    splitHosts(scyllaHosts),
		Keyspace: keyspace,
	})
	if err != nil {
		slog.Error("failed to connect to ScyllaDB", "error", err)
		os.Exit(1)
	}
	defer session.Close()

	secretsClient, err := openbaoClient.New(openbaoClient.Config{
		Address: openbaoAddr,
		Token:   openbaoToken,
	})
	if err != nil {
		slog.Error("failed to init OpenBao client", "error", err)
		os.Exit(1)
	}

	hostKubeconfig, err := loadHostKubeconfig(kubeConfigPath)
	if err != nil {
		slog.Error("failed to read host kubeconfig", "error", err)
		os.Exit(1)
	}

	vclusterClient, err := vcluster.New(hostKubeconfig)
	if err != nil {
		slog.Error("failed to init vcluster client", "error", err)
		os.Exit(1)
	}

	ciliumClient, err := cilium.New(hostKubeconfig)
	if err != nil {
		slog.Error("failed to init cilium client", "error", err)
		os.Exit(1)
	}

	gatewayClient, err := gateway.New(hostKubeconfig)
	if err != nil {
		slog.Error("failed to init gateway client", "error", err)
		os.Exit(1)
	}

	svc := service.New(service.Deps{
		VCluster:   vclusterClient,
		Cilium:     ciliumClient,
		Gateway:    gatewayClient,
		Kubeconfig: kubeconfig.New(secretsClient),
		CIDR:       cidr.New(session),
		Jobs:       jobs.New(session),
	})

	h := rest.NewHandler(svc)
	router := rest.NewRouter(h, internalSecret)

	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("cf-provisioner listening", "addr", addr, "keyspace", keyspace)
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
	return out
}

func loadHostKubeconfig(path string) ([]byte, error) {
	p := strings.TrimSpace(path)
	if p == "in-cluster" {
		return loadInClusterKubeconfig("/var/run/secrets/kubernetes.io/serviceaccount")
	}
	if p == "" && os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return loadInClusterKubeconfig("/var/run/secrets/kubernetes.io/serviceaccount")
	}
	if p == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		p = filepath.Join(home, ".kube", "config")
	}
	return os.ReadFile(p)
}

func loadInClusterKubeconfig(serviceAccountDir string) ([]byte, error) {
	host := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST"))
	port := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_PORT"))
	if host == "" {
		return nil, fmt.Errorf("KUBERNETES_SERVICE_HOST is required for in-cluster kubeconfig")
	}
	if port == "" {
		port = "443"
	}

	tokenPath := filepath.Join(serviceAccountDir, "token")
	caPath := filepath.Join(serviceAccountDir, "ca.crt")
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(caPath); err != nil {
		return nil, err
	}

	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: in-cluster
  cluster:
    certificate-authority: %q
    server: %q
users:
- name: serviceaccount
  user:
    token: %q
contexts:
- name: in-cluster
  context:
    cluster: in-cluster
    user: serviceaccount
current-context: in-cluster
`, caPath, "https://"+host+":"+port, strings.TrimSpace(string(token)))

	return []byte(kubeconfig), nil
}
