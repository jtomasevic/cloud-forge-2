package migrate_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/migrate"
)

// fakeQuerier is a test double that implements migrate.Querier without a
// running ScyllaDB. It records every statement passed to ExecCQL and allows
// tests to configure which filenames are "already applied".
type fakeQuerier struct {
	applied   []string // filenames to return from SelectStrings
	execCalls []string // all statements passed to ExecCQL, in call order
	execErr   error    // if set, ExecCQL returns this error
	selectErr error    // if set, SelectStrings returns this error
}

func (f *fakeQuerier) ExecCQL(_ context.Context, stmt string) error {
	if f.execErr != nil {
		return f.execErr
	}
	f.execCalls = append(f.execCalls, stmt)
	return nil
}

func (f *fakeQuerier) SelectStrings(_ context.Context, _ string) ([]string, error) {
	if f.selectErr != nil {
		return nil, f.selectErr
	}
	return f.applied, nil
}

// writeCQL writes a .cql file with the given content to dir.
func writeCQL(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

// ---------- Tests ----------

func TestRunMigrations_AppliesFilesInSortedOrder(t *testing.T) {
	dir := t.TempDir()

	// Write files out of alphabetical order; migration runner must sort them.
	writeCQL(t, dir, "002_create_tenants.cql", "CREATE TABLE IF NOT EXISTS tenants (id UUID PRIMARY KEY)")
	writeCQL(t, dir, "001_create_keyspace.cql", "CREATE KEYSPACE IF NOT EXISTS cloudforge WITH replication = {'class':'SimpleStrategy','replication_factor':'1'}")

	q := &fakeQuerier{}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: dir,
	})
	if err != nil {
		t.Fatalf("RunMigrations returned unexpected error: %v", err)
	}

	// The first ExecCQL call must be the schema_migrations CREATE TABLE.
	if len(q.execCalls) < 1 || !contains(q.execCalls[0], "schema_migrations") {
		t.Errorf("first ExecCQL call should create schema_migrations table, got: %q", firstOrEmpty(q.execCalls))
	}

	// Find the positions of the two migration statements.
	pos001 := indexOf(q.execCalls, "001_create_keyspace.cql")
	pos002 := indexOf(q.execCalls, "002_create_tenants.cql")

	if pos001 == -1 {
		t.Error("001_create_keyspace.cql was not applied")
	}
	if pos002 == -1 {
		t.Error("002_create_tenants.cql was not applied")
	}
	if pos001 != -1 && pos002 != -1 && pos001 > pos002 {
		t.Errorf("001 should be applied before 002 (got positions %d and %d)", pos001, pos002)
	}
}

func TestRunMigrations_SkipsAlreadyAppliedFiles(t *testing.T) {
	dir := t.TempDir()
	writeCQL(t, dir, "001_a.cql", "CREATE TABLE IF NOT EXISTS a (id UUID PRIMARY KEY)")
	writeCQL(t, dir, "002_b.cql", "CREATE TABLE IF NOT EXISTS b (id UUID PRIMARY KEY)")

	// Simulate that 001_a.cql was already applied.
	q := &fakeQuerier{applied: []string{"001_a.cql"}}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: dir,
	})
	if err != nil {
		t.Fatalf("RunMigrations returned unexpected error: %v", err)
	}

	// 001_a.cql's CREATE TABLE statement must NOT appear in execCalls.
	for _, call := range q.execCalls {
		if contains(call, "CREATE TABLE IF NOT EXISTS a") {
			t.Error("001_a.cql should have been skipped but its statement was executed")
		}
	}
	// 002_b.cql's statement MUST appear.
	found002 := false
	for _, call := range q.execCalls {
		if contains(call, "CREATE TABLE IF NOT EXISTS b") {
			found002 = true
		}
	}
	if !found002 {
		t.Error("002_b.cql should have been applied but its statement was not found")
	}
}

func TestRunMigrations_IdempotentWhenAllApplied(t *testing.T) {
	dir := t.TempDir()
	writeCQL(t, dir, "001_a.cql", "CREATE TABLE IF NOT EXISTS a (id UUID PRIMARY KEY)")

	// All files already applied.
	q := &fakeQuerier{applied: []string{"001_a.cql"}}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: dir,
	})
	if err != nil {
		t.Fatalf("RunMigrations returned unexpected error: %v", err)
	}

	// Only the CREATE TABLE schema_migrations call should have been made.
	for _, call := range q.execCalls {
		if contains(call, "CREATE TABLE IF NOT EXISTS a") {
			t.Error("already-applied migration statement should not have been executed")
		}
	}
}

func TestRunMigrations_IgnoresNonCQLFiles(t *testing.T) {
	dir := t.TempDir()
	writeCQL(t, dir, "001_a.cql", "CREATE TABLE IF NOT EXISTS a (id UUID PRIMARY KEY)")
	// Write a non-.cql file; it must be ignored.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	q := &fakeQuerier{}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: dir,
	})
	if err != nil {
		t.Fatalf("RunMigrations returned unexpected error: %v", err)
	}

	for _, call := range q.execCalls {
		if contains(call, "README") {
			t.Error("non-.cql file should have been ignored")
		}
	}
}

func TestRunMigrations_MultiStatementFile(t *testing.T) {
	dir := t.TempDir()
	// A single .cql file with multiple statements separated by semicolons.
	writeCQL(t, dir, "001_multi.cql",
		"CREATE TABLE IF NOT EXISTS a (id UUID PRIMARY KEY);\n"+
			"CREATE TABLE IF NOT EXISTS b (id UUID PRIMARY KEY);\n")

	q := &fakeQuerier{}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: dir,
	})
	if err != nil {
		t.Fatalf("RunMigrations returned unexpected error: %v", err)
	}

	countA, countB := 0, 0
	for _, call := range q.execCalls {
		if contains(call, "CREATE TABLE IF NOT EXISTS a") {
			countA++
		}
		if contains(call, "CREATE TABLE IF NOT EXISTS b") {
			countB++
		}
	}
	if countA != 1 {
		t.Errorf("expected table a statement once, got %d", countA)
	}
	if countB != 1 {
		t.Errorf("expected table b statement once, got %d", countB)
	}
}

func TestRunMigrations_SkipsUseStatements(t *testing.T) {
	dir := t.TempDir()
	writeCQL(t, dir, "001_use.cql",
		"USE cloudforge;\n"+
			"CREATE TABLE IF NOT EXISTS accounts (id UUID PRIMARY KEY);\n")

	q := &fakeQuerier{}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: dir,
	})
	if err != nil {
		t.Fatalf("RunMigrations returned unexpected error: %v", err)
	}

	for _, call := range q.execCalls {
		if strings.EqualFold(strings.TrimSpace(call), "USE cloudforge") {
			t.Error("USE statements should not be executed by the migration runner")
		}
	}
	if indexOf(q.execCalls, "CREATE TABLE IF NOT EXISTS accounts") == -1 {
		t.Error("non-USE migration statement was not executed")
	}
}

func TestRunMigrations_ReturnsErrorWhenExecFails(t *testing.T) {
	// ExecCQL fails immediately (on the CREATE TABLE schema_migrations call).
	dir := t.TempDir()
	writeCQL(t, dir, "001_a.cql", "CREATE TABLE IF NOT EXISTS a (id UUID PRIMARY KEY)")

	q := &fakeQuerier{execErr: errors.New("connection refused")}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: dir,
	})
	if err == nil {
		t.Error("expected an error when ExecCQL fails")
	}
}

func TestRunMigrations_ReturnsErrorWhenSelectFails(t *testing.T) {
	dir := t.TempDir()
	writeCQL(t, dir, "001_a.cql", "CREATE TABLE IF NOT EXISTS a (id UUID PRIMARY KEY)")

	q := &fakeQuerier{selectErr: errors.New("query timeout")}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: dir,
	})
	if err == nil {
		t.Error("expected an error when SelectStrings fails")
	}
}

func TestRunMigrations_ReturnsErrorForNonExistentScriptsDir(t *testing.T) {
	// os.ReadDir fails when the directory does not exist.
	q := &fakeQuerier{}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: "/nonexistent/path/to/migrations",
	})
	if err == nil {
		t.Error("expected an error for non-existent scripts directory")
	}
}

func TestRunMigrations_ReturnsErrorWhenFileUnreadable(t *testing.T) {
	// os.ReadFile fails when a .cql entry in the directory is a broken symlink.
	// ReadDir will include it, but ReadFile will fail to open the target.
	dir := t.TempDir()
	brokenLink := filepath.Join(dir, "001_broken.cql")
	if err := os.Symlink("/nonexistent/target.cql", brokenLink); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	q := &fakeQuerier{}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: dir,
	})
	if err == nil {
		t.Error("expected an error for unreadable migration file")
	}
}

// callLimitQuerier succeeds on ExecCQL for the first succeedFor calls, then
// returns laterErr. This lets tests target specific steps in the migration loop.
type callLimitQuerier struct {
	applied    []string
	execCalls  []string
	succeedFor int
	laterErr   error
}

func (f *callLimitQuerier) ExecCQL(_ context.Context, stmt string) error {
	if len(f.execCalls) >= f.succeedFor {
		return f.laterErr
	}
	f.execCalls = append(f.execCalls, stmt)
	return nil
}

func (f *callLimitQuerier) SelectStrings(_ context.Context, _ string) ([]string, error) {
	return f.applied, nil
}

func TestRunMigrations_ReturnsErrorWhenApplyMigrationFails(t *testing.T) {
	// ExecCQL call sequence for a single-statement file:
	//   call 1: CREATE TABLE schema_migrations  → success
	//   call 2: migration statement              → fail  (covered path)
	dir := t.TempDir()
	writeCQL(t, dir, "001_a.cql", "CREATE TABLE IF NOT EXISTS a (id UUID PRIMARY KEY)")

	q := &callLimitQuerier{succeedFor: 1, laterErr: errors.New("write timeout")}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: dir,
	})
	if err == nil {
		t.Error("expected an error when applying the migration statement fails")
	}
}

func TestRunMigrations_ReturnsErrorWhenRecordingMigrationFails(t *testing.T) {
	// ExecCQL call sequence for a single-statement file:
	//   call 1: CREATE TABLE schema_migrations  → success
	//   call 2: migration statement              → success
	//   call 3: INSERT INTO schema_migrations    → fail  (covered path)
	dir := t.TempDir()
	writeCQL(t, dir, "001_a.cql", "CREATE TABLE IF NOT EXISTS a (id UUID PRIMARY KEY)")

	q := &callLimitQuerier{succeedFor: 2, laterErr: errors.New("write timeout")}
	err := migrate.RunMigrations(context.Background(), migrate.MigrationConfig{
		Session:    q,
		ScriptsDir: dir,
	})
	if err == nil {
		t.Error("expected an error when recording the migration in schema_migrations fails")
	}
}

// ---------- Helpers ----------

// contains reports whether s contains substr.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// indexOf returns the index of the first element in slice that contains substr,
// or -1 if no element matches.
func indexOf(slice []string, substr string) int {
	for i, s := range slice {
		if strings.Contains(s, substr) {
			return i
		}
	}
	return -1
}

func firstOrEmpty(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}
