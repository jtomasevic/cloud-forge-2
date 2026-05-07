//go:build integration

package migrate_test

import (
	"context"
	"testing"

	scylladb "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
	"github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/migrate"
)

// newIntegrationSession creates a session connected to integrationScyllaAddr
// with the integration keyspace as the default. This ensures that unqualified
// table references (like schema_migrations created by RunMigrations) resolve to
// the correct keyspace.
// The session is closed automatically via t.Cleanup.
func newIntegrationSession(t *testing.T) *scylladb.Session {
	t.Helper()
	sess, err := scylladb.New(context.Background(), scylladb.Config{
		Hosts:    []string{integrationScyllaAddr},
		Keyspace: integrationKeyspace,
	})
	if err != nil {
		t.Fatalf("connect to ScyllaDB at %s: %v", integrationScyllaAddr, err)
	}
	t.Cleanup(sess.Close)
	return sess
}

// resetMigrationsTable drops schema_migrations in the integration keyspace,
// giving each test a clean slate. Since the session has a default keyspace,
// the unqualified name resolves correctly.
func resetMigrationsTable(t *testing.T, sess *scylladb.Session) {
	t.Helper()
	ctx := context.Background()
	if err := sess.ExecCQL(ctx, "DROP TABLE IF EXISTS schema_migrations"); err != nil {
		t.Fatalf("drop schema_migrations: %v", err)
	}
}

// ---------- RunMigrations integration tests ----------

func TestRunMigrations_Integration_AppliesAllFiles(t *testing.T) {
	ctx := context.Background()
	sess := newIntegrationSession(t)
	resetMigrationsTable(t, sess)

	err := migrate.RunMigrations(ctx, migrate.MigrationConfig{
		Session:    sess,
		ScriptsDir: "testdata/migrations",
	})
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// schema_migrations is created without a keyspace qualifier by RunMigrations,
	// so it lands in the session's default keyspace (integrationKeyspace).
	applied, err := sess.SelectStrings(ctx,
		"SELECT filename FROM schema_migrations")
	if err != nil {
		t.Fatalf("SelectStrings schema_migrations: %v", err)
	}
	if len(applied) != 2 {
		t.Errorf("expected 2 applied migrations, got %d: %v", len(applied), applied)
	}
}

func TestRunMigrations_Integration_Idempotent(t *testing.T) {
	ctx := context.Background()
	sess := newIntegrationSession(t)
	resetMigrationsTable(t, sess)

	cfg := migrate.MigrationConfig{
		Session:    sess,
		ScriptsDir: "testdata/migrations",
	}

	if err := migrate.RunMigrations(ctx, cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Second run must succeed without error and not duplicate rows.
	if err := migrate.RunMigrations(ctx, cfg); err != nil {
		t.Fatalf("second run (idempotency): %v", err)
	}

	applied, err := sess.SelectStrings(ctx, "SELECT filename FROM schema_migrations")
	if err != nil {
		t.Fatalf("SelectStrings: %v", err)
	}
	if len(applied) != 2 {
		t.Errorf("idempotent run duplicated entries — want 2 rows, got %d", len(applied))
	}
}

func TestRunMigrations_Integration_CreatedTablesExist(t *testing.T) {
	ctx := context.Background()
	sess := newIntegrationSession(t)
	resetMigrationsTable(t, sess)

	if err := migrate.RunMigrations(ctx, migrate.MigrationConfig{
		Session:    sess,
		ScriptsDir: "testdata/migrations",
	}); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Verify the tables created by the migration files actually exist by
	// inserting and reading a row.
	if err := sess.ExecCQL(ctx,
		"INSERT INTO "+integrationKeyspace+".items (id, name) VALUES ('i1', 'test-item')"); err != nil {
		t.Fatalf("insert into items: %v", err)
	}

	rows, err := sess.SelectStrings(ctx,
		"SELECT name FROM "+integrationKeyspace+".items WHERE id = 'i1'")
	if err != nil {
		t.Fatalf("select from items: %v", err)
	}
	if len(rows) != 1 || rows[0] != "test-item" {
		t.Errorf("unexpected rows: %v", rows)
	}
}
