package jobs

import (
	"errors"
	"testing"

	"github.com/gocql/gocql"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

func TestMapScanErr_NotFound(t *testing.T) {
	err := mapScanErr(gocql.ErrNotFound, ErrJobNotFound)
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound, got %v", err)
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

func TestJobStatusConstants(t *testing.T) {
	if string(JobStatusPending) != "pending" || string(JobStatusSucceeded) != "succeeded" {
		t.Fatal("job status constants drifted from provisioning_jobs.status text values")
	}
}
