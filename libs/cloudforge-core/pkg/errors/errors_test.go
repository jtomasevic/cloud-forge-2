package errors_test

import (
	"errors"
	"fmt"
	"testing"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

func TestNew(t *testing.T) {
	e := cferrors.New(cferrors.CodeNotFound, "resource missing")
	if e.Code() != cferrors.CodeNotFound {
		t.Errorf("expected code %q, got %q", cferrors.CodeNotFound, e.Code())
	}
	if e.Error() != "[NOT_FOUND] resource missing" {
		t.Errorf("unexpected Error() output: %q", e.Error())
	}
	if e.Unwrap() != nil {
		t.Error("expected nil Unwrap for error created with New")
	}
}

func TestWrap(t *testing.T) {
	cause := fmt.Errorf("original db error")
	e := cferrors.Wrap(cferrors.CodeInternal, "query failed", cause)

	if e.Code() != cferrors.CodeInternal {
		t.Errorf("expected code %q, got %q", cferrors.CodeInternal, e.Code())
	}
	if !errors.Is(e, cause) {
		t.Error("errors.Is should find the wrapped cause via Unwrap")
	}
	if e.Unwrap() != cause {
		t.Error("Unwrap should return the original cause")
	}
}

func TestErrorOutput(t *testing.T) {
	tests := []struct {
		code    cferrors.Code
		message string
		want    string
	}{
		{cferrors.CodeNotFound, "tenant not found", "[NOT_FOUND] tenant not found"},
		{cferrors.CodeForbidden, "access denied", "[FORBIDDEN] access denied"},
		{cferrors.CodeProvisioningFailed, "vcluster failed", "[PROVISIONING_FAILED] vcluster failed"},
	}

	for _, tc := range tests {
		e := cferrors.New(tc.code, tc.message)
		if e.Error() != tc.want {
			t.Errorf("Error() = %q, want %q", e.Error(), tc.want)
		}
	}
}

func TestWrapf(t *testing.T) {
	sentinel := cferrors.ErrNotFound
	err := cferrors.Wrapf(sentinel, "tenant %s does not exist", "t-123")

	if err.Code() != cferrors.CodeNotFound {
		t.Errorf("expected code %q, got %q", cferrors.CodeNotFound, err.Code())
	}
	if err.Error() != "[NOT_FOUND] tenant t-123 does not exist" {
		t.Errorf("unexpected message: %q", err.Error())
	}
	if !errors.Is(err, sentinel) {
		t.Error("errors.Is(err, sentinel) should be true — sentinel is in the cause chain")
	}
}

func TestWrapf_InheritsCodeFromSentinel(t *testing.T) {
	for _, sentinel := range []*cferrors.CFError{
		cferrors.ErrNotFound,
		cferrors.ErrForbidden,
		cferrors.ErrUnavailable,
		cferrors.ErrInternal,
	} {
		err := cferrors.Wrapf(sentinel, "host %s failed", "localhost")
		if err.Code() != sentinel.Code() {
			t.Errorf("Wrapf: code mismatch — got %q, want %q", err.Code(), sentinel.Code())
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("errors.Is should match sentinel %q", sentinel.Code())
		}
	}
}

func TestIs_SentinelMatchByCode(t *testing.T) {
	// A wrapped error with the same code as a sentinel must match via errors.Is.
	wrapped := cferrors.Wrap(cferrors.CodeNotFound, "account not found", fmt.Errorf("db miss"))
	if !errors.Is(wrapped, cferrors.ErrNotFound) {
		t.Error("errors.Is(wrapped, ErrNotFound) should be true when codes match")
	}
}

func TestIs_DoesNotMatchDifferentCode(t *testing.T) {
	e := cferrors.New(cferrors.CodeForbidden, "forbidden")
	if errors.Is(e, cferrors.ErrNotFound) {
		t.Error("errors.Is should return false when codes differ")
	}
}

func TestIs_NonCFErrorTarget(t *testing.T) {
	e := cferrors.New(cferrors.CodeInternal, "boom")
	plain := fmt.Errorf("plain error")
	if errors.Is(e, plain) {
		t.Error("CFError.Is should return false for non-CFError targets")
	}
}

func TestIs_ChainedWrapping(t *testing.T) {
	// Simulates a repository error being wrapped by a service layer error —
	// errors.Is should still resolve to the sentinel at the root.
	repoErr := cferrors.Wrap(cferrors.CodeNotFound, "row not found", fmt.Errorf("gocql: not found"))
	svcErr := cferrors.Wrap(cferrors.CodeNotFound, "tenant not found", repoErr)

	if !errors.Is(svcErr, cferrors.ErrNotFound) {
		t.Error("errors.Is should resolve through the chain to ErrNotFound")
	}
}

func TestAllSentinelsHaveCorrectCodes(t *testing.T) {
	cases := []struct {
		sentinel *cferrors.CFError
		code     cferrors.Code
	}{
		{cferrors.ErrNotFound, cferrors.CodeNotFound},
		{cferrors.ErrAlreadyExists, cferrors.CodeAlreadyExists},
		{cferrors.ErrInvalidInput, cferrors.CodeInvalidInput},
		{cferrors.ErrUnauthorized, cferrors.CodeUnauthorized},
		{cferrors.ErrForbidden, cferrors.CodeForbidden},
		{cferrors.ErrInternal, cferrors.CodeInternal},
		{cferrors.ErrUnavailable, cferrors.CodeUnavailable},
		{cferrors.ErrConflict, cferrors.CodeConflict},
	}
	for _, tc := range cases {
		if tc.sentinel.Code() != tc.code {
			t.Errorf("sentinel code mismatch: got %q, want %q", tc.sentinel.Code(), tc.code)
		}
	}
}
