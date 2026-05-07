//go:build integration

// Package migrate integration tests — TestMain starts a ScyllaDB container
// and pre-creates the keyspace used by all migrate integration tests.
//
// Run with:
//
//	go test -tags integration -v ./pkg/migrate/...
//
// Set SCYLLADB_HOST=host:port to use an existing instance instead of
// spinning up a container.
package migrate_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	scylladb "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// integrationScyllaAddr holds the host:port available to all migrate
// integration tests in this package. Set by TestMain.
var integrationScyllaAddr string

// integrationKeyspace is the dedicated keyspace for migrate integration tests.
// Pre-created in TestMain and dropped on teardown.
const integrationKeyspace = "cf_migrate_test"

func TestMain(m *testing.M) {
	ctx := context.Background()

	addr, ctr, err := startScyllaDBContainer(ctx)
	if err != nil {
		log.Fatalf("integration setup: %v", err)
	}
	integrationScyllaAddr = addr

	// Pre-create the integration keyspace so migration CQL files can reference
	// it without each test having to manage keyspace lifecycle.
	if err := createKeyspace(ctx, addr, integrationKeyspace); err != nil {
		_ = testcontainers.TerminateContainer(ctr)
		log.Fatalf("integration setup: create keyspace %q: %v", integrationKeyspace, err)
	}

	code := m.Run()

	// Drop the keyspace to clean up after all tests finish.
	dropKeyspace(ctx, addr, integrationKeyspace) //nolint:errcheck

	if err := testcontainers.TerminateContainer(ctr); err != nil {
		log.Printf("warning: failed to terminate ScyllaDB container: %v", err)
	}

	os.Exit(code)
}

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
		testcontainers.WithCmd("--developer-mode=1", "--smp=1"),
		testcontainers.WithWaitStrategy(
			wait.ForAll(
				wait.ForListeningPort("9042/tcp"),
				wait.ForLog("Starting listening for CQL clients"),
			).WithStartupTimeout(3*time.Minute),
		),
	)
	if err != nil {
		return "", nil, fmt.Errorf("run scylladb/scylla:6.2: %w", err)
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

func createKeyspace(ctx context.Context, addr, ks string) error {
	sess, err := scylladb.New(ctx, scylladb.Config{Hosts: []string{addr}})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer sess.Close()

	return sess.ExecCQL(ctx,
		`CREATE KEYSPACE IF NOT EXISTS `+ks+
			` WITH replication = {'class':'SimpleStrategy','replication_factor':1}`)
}

func dropKeyspace(ctx context.Context, addr, ks string) error {
	sess, err := scylladb.New(ctx, scylladb.Config{Hosts: []string{addr}})
	if err != nil {
		return err
	}
	defer sess.Close()
	return sess.ExecCQL(ctx, "DROP KEYSPACE IF EXISTS "+ks)
}
