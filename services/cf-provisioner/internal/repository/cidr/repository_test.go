package cidr

import (
	"errors"
	"testing"

	"github.com/gocql/gocql"
)

func TestMapScanErr_NotFound(t *testing.T) {
	err := mapScanErr(gocql.ErrNotFound, ErrCIDRNotFound)
	if !errors.Is(err, ErrCIDRNotFound) {
		t.Fatalf("expected ErrCIDRNotFound, got %v", err)
	}
}
