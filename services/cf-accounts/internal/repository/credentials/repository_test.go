package credentials

import (
	"errors"
	"testing"
	"time"

	"github.com/gocql/gocql"
)

func TestMapScanErr_NotFound(t *testing.T) {
	err := mapScanErr(gocql.ErrNotFound, ErrCredentialNotFound)
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("expected ErrCredentialNotFound, got %v", err)
	}
}

func TestErrIfCredentialRevoked(t *testing.T) {
	if err := errIfCredentialRevoked(time.Time{}); err != nil {
		t.Fatalf("zero time should mean active key, got %v", err)
	}
	if err := errIfCredentialRevoked(time.Unix(1, 0)); err == nil || !errors.Is(err, ErrCredentialRevoked) {
		t.Fatalf("expected ErrCredentialRevoked, got %v", err)
	}
}
