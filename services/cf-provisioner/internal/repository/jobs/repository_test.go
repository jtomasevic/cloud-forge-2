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

func TestAppServiceJobTypeConstants(t *testing.T) {
	got := []JobType{
		JobTypeCreateAppService,
		JobTypeDeleteAppService,
		JobTypeExposeAppService,
		JobTypeRemoveAppServiceExposure,
	}
	want := []string{
		"create_app_service",
		"delete_app_service",
		"expose_app_service",
		"remove_app_service_exposure",
	}
	for i := range want {
		if string(got[i]) != want[i] {
			t.Fatalf("app-service job type %d: got %q want %q", i, got[i], want[i])
		}
	}
}
