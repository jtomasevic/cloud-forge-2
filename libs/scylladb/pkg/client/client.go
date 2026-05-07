// Package client provides a thin wrapper around the gocql ScyllaDB driver.
// It handles session creation, default configuration, and exposes the minimal
// API surface needed by CloudForge services and the migration runner.
package client

import (
	"context"
	"time"

	"github.com/gocql/gocql"
	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

const (
	defaultTimeout  = 10 * time.Second
	defaultNumConns = 2
)

// Config holds the configuration for creating a ScyllaDB session.
type Config struct {
	// Hosts is the list of ScyllaDB contact points, e.g. ["localhost:9042"].
	Hosts []string
	// Keyspace is the default keyspace for all queries on this session.
	Keyspace string
	// Username and Password are optional credentials for authentication.
	Username string
	Password string
	// Timeout is the per-query timeout. Defaults to 10s when zero.
	Timeout time.Duration
	// NumConns is the number of connections per host. Defaults to 2 when zero.
	NumConns int
}

// Session wraps a gocql.Session and provides a typed, CloudForge-aware API.
type Session struct {
	inner *gocql.Session
}

// New creates a new connected ScyllaDB session using the provided Config.
// Returns ErrConnectionFailed (wrapped as a *cferrors.CFError) if the session
// cannot be established.
func New(_ context.Context, cfg Config) (*Session, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	numConns := cfg.NumConns
	if numConns == 0 {
		numConns = defaultNumConns
	}

	cluster := gocql.NewCluster(cfg.Hosts...)
	cluster.Keyspace = cfg.Keyspace
	cluster.Consistency = gocql.LocalQuorum
	cluster.Timeout = timeout
	cluster.NumConns = numConns

	if cfg.Username != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: cfg.Username,
			Password: cfg.Password,
		}
	}

	inner, err := cluster.CreateSession()
	if err != nil {
		return nil, cferrors.Wrap(cferrors.CodeUnavailable, "scylladb connection failed", err)
	}

	return &Session{inner: inner}, nil
}

// Query returns a gocql.Query for the given CQL statement and bound values.
// This is a direct pass-through to the underlying session for full flexibility.
func (s *Session) Query(stmt string, values ...interface{}) *gocql.Query {
	return s.inner.Query(stmt, values...)
}

// ExecCQL executes a CQL statement that returns no rows (DDL, INSERT, UPDATE,
// DELETE). The context is honoured for cancellation.
func (s *Session) ExecCQL(ctx context.Context, stmt string) error {
	return s.inner.Query(stmt).WithContext(ctx).Exec()
}

// SelectStrings executes a CQL query that returns a single TEXT column and
// collects all row values into a string slice. Used by the migration runner to
// list already-applied migration filenames.
func (s *Session) SelectStrings(ctx context.Context, stmt string) ([]string, error) {
	iter := s.inner.Query(stmt).WithContext(ctx).Iter()
	var results []string
	var val string
	for iter.Scan(&val) {
		results = append(results, val)
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return results, nil
}

// Close releases all resources held by the session. It is safe to call
// multiple times.
func (s *Session) Close() {
	s.inner.Close()
}
