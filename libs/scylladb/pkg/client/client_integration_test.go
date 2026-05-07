//go:build integration

package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// sharedKeyspace is created once in TestMain (via createIntegrationKeyspace)
// and shared across all client integration tests.
const sharedKeyspace = "cf_client_test"

// integrationSession opens a session to integrationScyllaAddr, creates the
// shared keyspace and returns the session. The caller is responsible for
// calling sess.Close() (typically via t.Cleanup).
func integrationSession(t *testing.T) *Session {
	t.Helper()
	ctx := context.Background()

	sess, err := New(ctx, Config{Hosts: []string{integrationScyllaAddr}})
	if err != nil {
		t.Fatalf("connect to ScyllaDB at %s: %v", integrationScyllaAddr, err)
	}
	t.Cleanup(sess.Close)

	// Ensure the shared keyspace exists.
	mustExec(t, ctx, sess,
		`CREATE KEYSPACE IF NOT EXISTS `+sharedKeyspace+
			` WITH replication = {'class':'SimpleStrategy','replication_factor':1}`)

	return sess
}

// ---------- New() ----------

func TestNew_Integration_ConnectsAndCloses(t *testing.T) {
	ctx := context.Background()
	sess, err := New(ctx, Config{Hosts: []string{integrationScyllaAddr}})
	if err != nil {
		t.Fatalf("expected successful connection, got: %v", err)
	}
	sess.Close()
}

func TestNew_Integration_WithDefaultsReturnsConnectedSession(t *testing.T) {
	// Timeout == 0 and NumConns == 0 → defaults applied, connection should succeed.
	ctx := context.Background()
	sess, err := New(ctx, Config{
		Hosts:    []string{integrationScyllaAddr},
		Timeout:  0,
		NumConns: 0,
	})
	if err != nil {
		t.Fatalf("expected defaults to work, got: %v", err)
	}
	t.Cleanup(sess.Close)
}

func TestNew_Integration_FailsForUnreachableHost(t *testing.T) {
	_, err := New(context.Background(), Config{Hosts: []string{"127.0.0.1:1"}})
	if err == nil {
		t.Fatal("expected connection error for unreachable host")
	}
	if !errors.Is(err, ErrConnectionFailed) {
		t.Errorf("expected ErrConnectionFailed, got %T: %v", err, err)
	}
}

// ---------- ExecCQL ----------

func TestExecCQL_Integration_DDLAndDML(t *testing.T) {
	ctx := context.Background()
	sess := integrationSession(t)

	table := uniqueTable(t)

	mustExec(t, ctx, sess, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s.%s (id text PRIMARY KEY, val text)`,
		sharedKeyspace, table))
	t.Cleanup(func() {
		sess.ExecCQL(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s.%s", sharedKeyspace, table)) //nolint:errcheck
	})

	mustExec(t, ctx, sess, fmt.Sprintf(
		`INSERT INTO %s.%s (id, val) VALUES ('k1', 'v1')`, sharedKeyspace, table))
}

func TestExecCQL_Integration_ReturnsErrorForBadCQL(t *testing.T) {
	sess := integrationSession(t)
	err := sess.ExecCQL(context.Background(), "THIS IS NOT VALID CQL")
	if err == nil {
		t.Fatal("expected error for invalid CQL statement, got nil")
	}
}

// ---------- SelectStrings ----------

func TestSelectStrings_Integration_ReturnsInsertedRows(t *testing.T) {
	ctx := context.Background()
	sess := integrationSession(t)

	table := uniqueTable(t)

	mustExec(t, ctx, sess, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s.%s (name text PRIMARY KEY)`,
		sharedKeyspace, table))
	t.Cleanup(func() {
		sess.ExecCQL(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s.%s", sharedKeyspace, table)) //nolint:errcheck
	})

	mustExec(t, ctx, sess, fmt.Sprintf(`INSERT INTO %s.%s (name) VALUES ('alpha')`, sharedKeyspace, table))
	mustExec(t, ctx, sess, fmt.Sprintf(`INSERT INTO %s.%s (name) VALUES ('beta')`, sharedKeyspace, table))

	rows, err := sess.SelectStrings(ctx,
		fmt.Sprintf(`SELECT name FROM %s.%s`, sharedKeyspace, table))
	if err != nil {
		t.Fatalf("SelectStrings: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d: %v", len(rows), rows)
	}
}

func TestSelectStrings_Integration_ReturnsEmptySliceForNoRows(t *testing.T) {
	ctx := context.Background()
	sess := integrationSession(t)

	table := uniqueTable(t)

	mustExec(t, ctx, sess, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s.%s (name text PRIMARY KEY)`,
		sharedKeyspace, table))
	t.Cleanup(func() {
		sess.ExecCQL(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s.%s", sharedKeyspace, table)) //nolint:errcheck
	})

	rows, err := sess.SelectStrings(ctx,
		fmt.Sprintf(`SELECT name FROM %s.%s`, sharedKeyspace, table))
	if err != nil {
		t.Fatalf("SelectStrings: %v", err)
	}
	if rows != nil && len(rows) != 0 {
		t.Errorf("expected empty result, got %v", rows)
	}
}

// ---------- Query (raw) ----------

func TestQuery_Integration_Scan(t *testing.T) {
	ctx := context.Background()
	sess := integrationSession(t)

	table := uniqueTable(t)

	mustExec(t, ctx, sess, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s.%s (name text PRIMARY KEY)`,
		sharedKeyspace, table))
	t.Cleanup(func() {
		sess.ExecCQL(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s.%s", sharedKeyspace, table)) //nolint:errcheck
	})

	mustExec(t, ctx, sess, fmt.Sprintf(
		`INSERT INTO %s.%s (name) VALUES ('hello')`, sharedKeyspace, table))

	var name string
	if err := sess.Query(
		fmt.Sprintf(`SELECT name FROM %s.%s WHERE name = ?`, sharedKeyspace, table),
		"hello",
	).WithContext(ctx).Scan(&name); err != nil {
		t.Fatalf("Query.Scan: %v", err)
	}
	if name != "hello" {
		t.Errorf("expected 'hello', got %q", name)
	}
}

// ---------- helpers ----------

// uniqueTable derives a ScyllaDB-safe table name (lowercase, underscores only)
// from the test name, ensuring isolation between tests.
func uniqueTable(t *testing.T) string {
	t.Helper()
	name := strings.ToLower(t.Name())
	name = strings.NewReplacer("/", "_", "-", "_", " ", "_").Replace(name)
	// ScyllaDB identifiers max 48 chars; "test" prefix is safe
	if len(name) > 48 {
		name = name[:48]
	}
	return name
}

func mustExec(t *testing.T, ctx context.Context, s *Session, stmt string) {
	t.Helper()
	if err := s.ExecCQL(ctx, stmt); err != nil {
		t.Fatalf("ExecCQL(%q): %v", stmt, err)
	}
}
