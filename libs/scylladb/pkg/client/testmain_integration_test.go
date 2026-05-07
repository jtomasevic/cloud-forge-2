//go:build integration

// Package client integration tests — this file owns TestMain which starts a
// ScyllaDB container once for the whole package test run.
//
// Run with:
//
//	go test -tags integration -v ./pkg/client/...
//
// Set SCYLLADB_HOST=host:port to skip container startup and use an existing
// instance (useful with Option-A Makefile targets).
package client

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// integrationScyllaAddr is the host:port of the ScyllaDB instance available
// for all integration tests in this package.
var integrationScyllaAddr string

func TestMain(m *testing.M) {
	ctx := context.Background()

	addr, ctr, err := startScyllaDBContainer(ctx)
	if err != nil {
		log.Fatalf("integration setup: failed to start ScyllaDB: %v", err)
	}
	integrationScyllaAddr = addr

	code := m.Run()

	// nil-safe: TerminateContainer handles nil containers gracefully.
	if err := testcontainers.TerminateContainer(ctr); err != nil {
		log.Printf("warning: failed to terminate ScyllaDB container: %v", err)
	}

	os.Exit(code)
}

// startScyllaDBContainer starts a single ScyllaDB container using
// testcontainers-go and waits until CQL port 9042 is accepting connections.
//
// If the environment variable SCYLLADB_HOST is set the function returns that
// address immediately and skips container creation (Option-A / CI with an
// externally-managed cluster).
func startScyllaDBContainer(ctx context.Context) (addr string, ctr *testcontainers.DockerContainer, err error) {
	if h := os.Getenv("SCYLLADB_HOST"); h != "" {
		log.Printf("SCYLLADB_HOST is set — using external ScyllaDB at %s", h)
		return h, nil, nil
	}

	log.Println("starting ScyllaDB container via testcontainers-go…")

	ctr, err = testcontainers.Run(
		ctx,
		"scylladb/scylla:6.2",
		testcontainers.WithExposedPorts("9042/tcp"),
		// developer-mode + single shard → fast startup, low resource usage
		testcontainers.WithCmd("--developer-mode=1", "--smp=1"),
		testcontainers.WithWaitStrategy(
			wait.ForAll(
				// Port must be open…
				wait.ForListeningPort("9042/tcp"),
				// …and ScyllaDB must have printed its ready message.
				wait.ForLog("Starting listening for CQL clients"),
			).WithStartupTimeout(3*time.Minute),
		),
	)
	if err != nil {
		return "", nil, fmt.Errorf("run scylladb/scylla:6.2 container: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		return "", nil, fmt.Errorf("get container host: %w", err)
	}

	port, err := ctr.MappedPort(ctx, "9042")
	if err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		return "", nil, fmt.Errorf("get mapped port: %w", err)
	}

	addr = fmt.Sprintf("%s:%s", host, port.Port())
	log.Printf("ScyllaDB container ready at %s", addr)
	return addr, ctr, nil
}
