// Package migrate provides a simple, idempotent CQL schema migration runner.
// Migrations are plain .cql files stored in a directory and applied in
// lexicographic filename order. Each successfully applied file is recorded in
// the schema_migrations table so that re-running is always safe.
package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

// Querier is the minimal database interface required by the migration runner.
// *client.Session satisfies this interface, and tests can provide a lightweight
// fake without a running ScyllaDB instance.
type Querier interface {
	// ExecCQL executes a CQL statement that returns no rows (DDL, INSERT, etc.).
	ExecCQL(ctx context.Context, stmt string) error
	// SelectStrings executes a CQL query that returns a single TEXT column and
	// collects all values into a string slice.
	SelectStrings(ctx context.Context, stmt string) ([]string, error)
}

// MigrationConfig holds the parameters for RunMigrations.
type MigrationConfig struct {
	// Session is the database connection used to apply migrations.
	Session Querier
	// ScriptsDir is the path to the directory containing .cql migration files.
	ScriptsDir string
}

const createMigrationsTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
    filename TEXT PRIMARY KEY,
    applied_at TIMESTAMP
)`

// RunMigrations applies all .cql files in cfg.ScriptsDir that have not yet
// been recorded in the schema_migrations table. Files are applied in
// lexicographic filename order, ensuring a deterministic migration sequence.
//
// RunMigrations is idempotent: files already present in schema_migrations are
// silently skipped. A failure mid-run leaves the table in a partially applied
// state; re-running will skip already-applied files and resume from the point
// of failure.
func RunMigrations(ctx context.Context, cfg MigrationConfig) error {
	// Ensure the tracking table exists before querying it.
	if err := cfg.Session.ExecCQL(ctx, createMigrationsTable); err != nil {
		return cferrors.Wrap(cferrors.CodeInternal, "failed to create schema_migrations table", err)
	}

	// Load the set of already-applied migration filenames.
	applied, err := cfg.Session.SelectStrings(ctx, "SELECT filename FROM schema_migrations")
	if err != nil {
		return cferrors.Wrap(cferrors.CodeInternal, "failed to query schema_migrations", err)
	}
	appliedSet := make(map[string]struct{}, len(applied))
	for _, name := range applied {
		appliedSet[name] = struct{}{}
	}

	// Collect and sort .cql files.
	entries, err := os.ReadDir(cfg.ScriptsDir)
	if err != nil {
		return cferrors.Wrap(cferrors.CodeInternal, "failed to read migrations directory", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".cql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	// Apply each file that has not been applied yet.
	for _, filename := range files {
		if _, ok := appliedSet[filename]; ok {
			continue
		}

		path := filepath.Join(cfg.ScriptsDir, filename)
		content, err := os.ReadFile(path)
		if err != nil {
			return cferrors.Wrap(cferrors.CodeInternal,
				fmt.Sprintf("failed to read migration file %s", filename), err)
		}

		statements := splitStatements(string(content))
		for _, stmt := range statements {
			if err := cfg.Session.ExecCQL(ctx, stmt); err != nil {
				return cferrors.Wrap(cferrors.CodeInternal,
					fmt.Sprintf("failed to apply migration %s", filename), err)
			}
		}

		// Record the file as applied.
		record := fmt.Sprintf(
			"INSERT INTO schema_migrations (filename, applied_at) VALUES ('%s', '%s')",
			filename,
			time.Now().UTC().Format(time.RFC3339),
		)
		if err := cfg.Session.ExecCQL(ctx, record); err != nil {
			return cferrors.Wrap(cferrors.CodeInternal,
				fmt.Sprintf("failed to record migration %s", filename), err)
		}
	}

	return nil
}

// splitStatements splits a CQL script on semicolons, returning each non-empty
// statement. This handles files that contain multiple statements separated by
// semicolons (the standard CQL convention).
func splitStatements(script string) []string {
	parts := strings.Split(script, ";")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			result = append(result, s)
		}
	}
	return result
}
