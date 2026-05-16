package tenants

import (
	"errors"
	"testing"

	"github.com/gocql/gocql"
)

func TestMapScanErr_NotFound(t *testing.T) {
	err := mapScanErr(gocql.ErrNotFound, ErrTenantNotFound)
	if !errors.Is(err, ErrTenantNotFound) {
		t.Fatalf("expected ErrTenantNotFound, got %v", err)
	}
}
