// Package client white-box tests.
// Uses package client (not client_test) to access unexported helpers:
// scanStringIter and the gocqlIter interface.
package client

import (
	"context"
	"errors"
	"testing"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

// ---------- New() — error-path tests (no live ScyllaDB required) ----------
//
// All tests use an unreachable address (127.0.0.1:1) so gocql fails fast
// with "connection refused" instead of a long timeout. This exercises every
// branch in New() without requiring a running cluster.

func TestNew_DefaultTimeoutAndNumConns(t *testing.T) {
	// cfg.Timeout == 0 and cfg.NumConns == 0 → defaults applied before failing.
	_, err := New(context.Background(), Config{
		Hosts: []string{"127.0.0.1:1"}, // unreachable — connection refused
	})
	if err == nil {
		t.Fatal("expected error for unreachable host, got nil")
	}
	if !errors.Is(err, ErrConnectionFailed) {
		t.Errorf("expected ErrConnectionFailed, got %T: %v", err, err)
	}
	if !errors.Is(err, cferrors.ErrUnavailable) {
		t.Errorf("expected CodeUnavailable in error chain, got %v", err)
	}
}

func TestNew_ExplicitTimeoutAndNumConns(t *testing.T) {
	// cfg.Timeout != 0 and cfg.NumConns != 0 → use provided values.
	_, err := New(context.Background(), Config{
		Hosts:    []string{"127.0.0.1:1"},
		Timeout:  50, // 50 ns — any non-zero value exercises the non-default branch
		NumConns: 1,
	})
	if err == nil {
		t.Fatal("expected error for unreachable host, got nil")
	}
	if !errors.Is(err, ErrConnectionFailed) {
		t.Errorf("expected ErrConnectionFailed, got %v", err)
	}
}

func TestNewWithInner_ReturnsNonNilSession(t *testing.T) {
	// newWithInner is used by integration tests that provide their own *gocql.Session.
	// Passing nil is valid here because no methods are called on the session.
	s := newWithInner(nil)
	if s == nil {
		t.Fatal("newWithInner should return a non-nil *Session")
	}
}

func TestNew_WithCredentials(t *testing.T) {
	// cfg.Username != "" → PasswordAuthenticator branch is exercised.
	_, err := New(context.Background(), Config{
		Hosts:    []string{"127.0.0.1:1"},
		Username: "cassandra",
		Password: "cassandra",
	})
	if err == nil {
		t.Fatal("expected error for unreachable host, got nil")
	}
	if !errors.Is(err, ErrConnectionFailed) {
		t.Errorf("expected ErrConnectionFailed, got %v", err)
	}
}

// ---------- scanStringIter — unit tests via fake gocqlIter ----------

// fakeIter implements gocqlIter for testing scanStringIter without a live DB.
type fakeIter struct {
	rows     []string // values to return from Scan, one per call
	pos      int      // current position in rows
	closeErr error    // error to return from Close
}

func (f *fakeIter) Scan(dest ...interface{}) bool {
	if f.pos >= len(f.rows) {
		return false
	}
	if sp, ok := dest[0].(*string); ok {
		*sp = f.rows[f.pos]
	}
	f.pos++
	return true
}

func (f *fakeIter) Close() error {
	return f.closeErr
}

func TestScanStringIter_ReturnsAllRows(t *testing.T) {
	iter := &fakeIter{rows: []string{"001_a.cql", "002_b.cql", "003_c.cql"}}
	got, err := scanStringIter(iter)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 rows, got %d: %v", len(got), got)
	}
	if got[0] != "001_a.cql" || got[1] != "002_b.cql" || got[2] != "003_c.cql" {
		t.Errorf("unexpected rows: %v", got)
	}
}

func TestScanStringIter_EmptyResultSet(t *testing.T) {
	iter := &fakeIter{rows: []string{}}
	got, err := scanStringIter(iter)
	if err != nil {
		t.Fatalf("expected nil error for empty result, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestScanStringIter_PropagatesCloseError(t *testing.T) {
	iter := &fakeIter{
		rows:     []string{"row1"},
		closeErr: errors.New("network reset during iteration"),
	}
	_, err := scanStringIter(iter)
	if err == nil {
		t.Fatal("expected error from iter.Close(), got nil")
	}
	if err.Error() != "network reset during iteration" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestScanStringIter_CloseErrorWithNoRows(t *testing.T) {
	// Close fails even when no rows were scanned.
	iter := &fakeIter{
		rows:     []string{},
		closeErr: errors.New("connection lost"),
	}
	_, err := scanStringIter(iter)
	if err == nil {
		t.Fatal("expected error from iter.Close(), got nil")
	}
}
