//go:build integration

// Package client integration tests — this file owns TestMain which starts an
// OpenBao / Vault-compatible container once for the whole package test run.
//
// Run with:
//
//	go test -tags integration -v ./pkg/client/...
//
// Set OPENBAO_ADDR=http://host:8200 and OPENBAO_TOKEN=<token> to skip
// container startup and use an existing instance instead.
package client

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	api "github.com/openbao/openbao/api/v2"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// integrationBaoAddr and integrationBaoToken are set by TestMain and shared
// across all integration tests in this package.
var (
	integrationBaoAddr  string
	integrationBaoToken string
)

// devToken is the fixed root token used when we start our own container.
const devToken = "root"

func TestMain(m *testing.M) {
	ctx := context.Background()

	addr, token, ctr, err := startOpenBaoContainer(ctx)
	if err != nil {
		log.Fatalf("integration setup: failed to start OpenBao: %v", err)
	}
	integrationBaoAddr = addr
	integrationBaoToken = token

	code := m.Run()

	// nil-safe: TerminateContainer handles nil containers gracefully (e.g.
	// when OPENBAO_ADDR was set and no container was started).
	if err := testcontainers.TerminateContainer(ctr); err != nil {
		log.Printf("warning: failed to terminate OpenBao container: %v", err)
	}

	os.Exit(code)
}

// startOpenBaoContainer starts a Vault-compatible dev-mode container using
// testcontainers-go and waits until the health endpoint returns 200.
//
// If OPENBAO_ADDR is set the function returns that address (and OPENBAO_TOKEN
// for the token) immediately, skipping container creation.
func startOpenBaoContainer(ctx context.Context) (addr, token string, ctr *testcontainers.DockerContainer, err error) {
	if h := os.Getenv("OPENBAO_ADDR"); h != "" {
		t := os.Getenv("OPENBAO_TOKEN")
		if t == "" {
			t = devToken
		}
		log.Printf("OPENBAO_ADDR is set — using external OpenBao at %s", h)
		return h, t, nil, nil
	}

	log.Println("starting OpenBao (Vault-compatible) container via testcontainers-go…")

	// hashicorp/vault in -dev mode is wire-compatible with the OpenBao SDK and
	// starts with:
	//   • KV v1 mount at secret/       (matches kubeconfigPathPrefix)
	//   • root token fixed to devToken
	//   • no TLS, no persistence
	ctr, err = testcontainers.Run(
		ctx,
		"hashicorp/vault:1.15.6",
		testcontainers.WithExposedPorts("8200/tcp"),
		testcontainers.WithEnv(map[string]string{
			"VAULT_DEV_ROOT_TOKEN_ID":  devToken,
			"VAULT_DEV_LISTEN_ADDRESS": "0.0.0.0:8200",
		}),
		testcontainers.WithCmd("server", "-dev"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithPort("8200/tcp").
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return "", "", nil, fmt.Errorf("run vault:1.15.6 container: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		return "", "", nil, fmt.Errorf("get container host: %w", err)
	}

	port, err := ctr.MappedPort(ctx, "8200")
	if err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		return "", "", nil, fmt.Errorf("get mapped port: %w", err)
	}

	addr = fmt.Sprintf("http://%s:%s", host, port.Port())
	log.Printf("OpenBao container ready at %s (token: %s)", addr, devToken)

	// Vault dev mode mounts secret/ as KV v2 since Vault ≥1.12. Our production
	// code uses KV v1 paths (secret/tenants/…), so we remount it as KV v1 now.
	if err := remountSecretAsKVv1(ctx, addr, devToken); err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		return "", "", nil, fmt.Errorf("remount secret/ as KV v1: %w", err)
	}
	log.Println("secret/ remounted as KV v1")

	return addr, devToken, ctr, nil
}

// remountSecretAsKVv1 disables the default KV v2 mount at secret/ and
// re-enables it as KV v1, which matches the paths used by our production code.
func remountSecretAsKVv1(ctx context.Context, addr, token string) error {
	vaultCfg := api.DefaultConfig()
	vaultCfg.Address = addr

	c, err := api.NewClient(vaultCfg)
	if err != nil {
		return fmt.Errorf("create api client: %w", err)
	}
	c.SetToken(token)

	if err := c.Sys().UnmountWithContext(ctx, "secret"); err != nil {
		return fmt.Errorf("unmount secret/: %w", err)
	}
	if err := c.Sys().MountWithContext(ctx, "secret", &api.MountInput{
		Type:    "kv",
		Options: map[string]string{"version": "1"},
	}); err != nil {
		return fmt.Errorf("mount secret/ as kv v1: %w", err)
	}
	return nil
}

// integrationClient creates a real CFSecretsClient connected to the
// integration instance. Fails the test immediately on connection error.
func integrationClient(t *testing.T) SecretsClient {
	t.Helper()
	c, err := New(Config{
		Address: integrationBaoAddr,
		Token:   integrationBaoToken,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}
