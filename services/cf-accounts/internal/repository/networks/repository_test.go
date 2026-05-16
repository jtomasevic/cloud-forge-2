package networks

import (
	"errors"
	"testing"

	"github.com/gocql/gocql"
)

func TestMapScanErr_NotFound(t *testing.T) {
	err := mapScanErr(gocql.ErrNotFound, ErrNetworkNotFound)
	if !errors.Is(err, ErrNetworkNotFound) {
		t.Fatalf("expected ErrNetworkNotFound, got %v", err)
	}
}
