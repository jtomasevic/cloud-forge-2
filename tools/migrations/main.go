package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
	"github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/migrate"
)

func main() {
	hosts := flag.String("hosts", "localhost:9042", "comma-separated ScyllaDB hosts")
	keyspace := flag.String("keyspace", "cloudforge", "target keyspace")
	scriptsDir := flag.String("scripts-dir", "./scripts", "directory containing .cql migration files")
	flag.Parse()

	ctx := context.Background()
	hostList := splitAndTrimHosts(*hosts)
	if len(hostList) == 0 {
		log.Fatal("no hosts given (use --hosts host:port or comma-separated list)")
	}

	// gocql cannot use a default keyspace until it exists. Bootstrap with no
	// keyspace, then connect with --keyspace for RunMigrations (which records
	// schema_migrations in that keyspace).
	fmt.Printf("ensuring keyspace %q exists...\n", *keyspace)
	bootstrap, err := client.New(ctx, client.Config{Hosts: hostList})
	if err != nil {
		log.Fatalf("failed to connect to ScyllaDB (bootstrap): %v", err)
	}
	createKS := fmt.Sprintf(
		"CREATE KEYSPACE IF NOT EXISTS %s WITH replication = {'class': 'SimpleStrategy', 'replication_factor': '1'} AND durable_writes = true",
		*keyspace,
	)
	if err := bootstrap.ExecCQL(ctx, createKS); err != nil {
		bootstrap.Close()
		log.Fatalf("failed to create keyspace: %v", err)
	}
	bootstrap.Close()

	cfg := client.Config{
		Hosts:    hostList,
		Keyspace: *keyspace,
	}

	fmt.Printf("connecting to %v (keyspace %q)...\n", hostList, *keyspace)
	session, err := client.New(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to connect to ScyllaDB: %v", err)
	}
	defer session.Close()

	fmt.Printf("applying migrations from %s...\n", *scriptsDir)
	if err := migrate.RunMigrations(ctx, migrate.MigrationConfig{
		Session:    session,
		ScriptsDir: *scriptsDir,
	}); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	fmt.Println("migrations applied successfully")
}

func splitAndTrimHosts(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
