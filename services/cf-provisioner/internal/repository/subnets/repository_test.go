package subnets

import (
	"errors"
	"strings"
	"testing"

	"github.com/gocql/gocql"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

func TestMapScanErr_NotFound(t *testing.T) {
	err := mapScanErr(gocql.ErrNotFound, ErrSubnetNotFound)
	if !errors.Is(err, ErrSubnetNotFound) {
		t.Fatalf("expected ErrSubnetNotFound, got %v", err)
	}
}

func TestParseUUID_Invalid(t *testing.T) {
	_, err := parseUUID("not-a-uuid")
	if err == nil {
		t.Fatal("expected error")
	}
	var ce *cferrors.CFError
	if !errors.As(err, &ce) || ce.Code() != cferrors.CodeInvalidInput {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestParseRequiredUUID_Missing(t *testing.T) {
	_, err := parseRequiredUUID("networkID", " ")
	if err == nil {
		t.Fatal("expected error")
	}
	var ce *cferrors.CFError
	if !errors.As(err, &ce) || ce.Code() != cferrors.CodeInvalidInput {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestNormalizeType(t *testing.T) {
	got, err := normalizeType(" PUBLIC ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "public" {
		t.Fatalf("type: got %q", got)
	}
	if _, err := normalizeType("dmz"); err == nil {
		t.Fatal("expected invalid subnet type")
	}
}

func TestNormalizeCIDR(t *testing.T) {
	got, err := normalizeCIDR("10.10.1.8/24")
	if err != nil {
		t.Fatal(err)
	}
	if got != "10.10.1.0/24" {
		t.Fatalf("canonical CIDR: got %q", got)
	}
	if _, err := normalizeCIDR("2001:db8::/64"); err == nil {
		t.Fatal("expected IPv6 CIDR rejection")
	}
}

func TestSubnetErrorsHaveStableCodes(t *testing.T) {
	var exists *cferrors.CFError
	if !errors.As(ErrSubnetCIDRExists, &exists) || exists.Code() != cferrors.CodeAlreadyExists {
		t.Fatalf("duplicate CIDR error code drifted: %v", ErrSubnetCIDRExists)
	}
	var notFound *cferrors.CFError
	if !errors.As(ErrSubnetNotFound, &notFound) || notFound.Code() != cferrors.CodeNotFound {
		t.Fatalf("not found error code drifted: %v", ErrSubnetNotFound)
	}
}

func TestDuplicateCIDRCommandUsesLWT(t *testing.T) {
	if !strings.Contains(cqlInsertSubnetByNetworkCIDR, "IF NOT EXISTS") {
		t.Fatalf("duplicate CIDR insert must reserve with LWT: %s", cqlInsertSubnetByNetworkCIDR)
	}
}
