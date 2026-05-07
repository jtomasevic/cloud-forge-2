// Package errors defines the platform-wide typed error system for CloudForge.
// Every CF service uses CFError as its base error type. Each layer (REST,
// service, repository) defines its own typed errors in its own errors.go but
// all ultimately wrap or return *CFError values so that error codes are
// consistent across layer boundaries.
package errors

import "fmt"

// Code is a machine-readable error classification used across all CF services.
type Code string

const (
	// CodeNotFound indicates the requested resource does not exist.
	CodeNotFound Code = "NOT_FOUND"
	// CodeAlreadyExists indicates a resource with conflicting identity already exists.
	CodeAlreadyExists Code = "ALREADY_EXISTS"
	// CodeInvalidInput indicates the caller provided invalid or malformed input.
	CodeInvalidInput Code = "INVALID_INPUT"
	// CodeUnauthorized indicates the caller is not authenticated.
	CodeUnauthorized Code = "UNAUTHORIZED"
	// CodeForbidden indicates the caller is authenticated but not permitted.
	CodeForbidden Code = "FORBIDDEN"
	// CodeInternal indicates an unexpected internal error.
	CodeInternal Code = "INTERNAL"
	// CodeUnavailable indicates a downstream dependency is unreachable.
	CodeUnavailable Code = "UNAVAILABLE"
	// CodeConflict indicates a state conflict prevented the operation.
	CodeConflict Code = "CONFLICT"
	// CodeProvisioningFailed indicates an infrastructure provisioning operation failed.
	CodeProvisioningFailed Code = "PROVISIONING_FAILED"
)

// CFError is the base typed error for CloudForge platform errors.
// It always carries a Code and a human-readable message, and optionally
// the underlying cause. The cause is preserved for debugging but must
// never be surfaced to external (HTTP) callers.
type CFError struct {
	code    Code
	message string
	cause   error
}

// New creates a new CFError with the given code and message and no cause.
func New(code Code, message string) *CFError {
	return &CFError{code: code, message: message}
}

// Wrap creates a new CFError with the given code and message, wrapping cause
// so that the original error is preserved for logging and errors.Unwrap.
func Wrap(code Code, message string, cause error) *CFError {
	return &CFError{code: code, message: message, cause: cause}
}

// Wrapf wraps sentinel with a printf-style message carrying runtime context.
// The code is inherited from sentinel so callers never repeat it.
// The returned error satisfies errors.Is(err, sentinel) because sentinel is
// placed in the cause chain.
//
// Use this whenever a sentinel error needs runtime values in its message:
//
//	return cferrors.Wrapf(ErrNotFound, "tenant %s does not exist", id)
//	return cferrors.Wrapf(ErrUnavailable, "host %s unreachable: %v", host, err)
func Wrapf(sentinel *CFError, format string, args ...any) *CFError {
	return Wrap(sentinel.Code(), fmt.Sprintf(format, args...), sentinel)
}

// Code returns the machine-readable error classification.
func (e *CFError) Code() Code { return e.code }

// Error implements the error interface.
func (e *CFError) Error() string { return fmt.Sprintf("[%s] %s", e.code, e.message) }

// Unwrap returns the underlying cause, enabling errors.Is and errors.As
// to traverse the error chain.
func (e *CFError) Unwrap() error { return e.cause }

// Is supports errors.Is comparisons by Code so that callers can test
// whether any error in the chain has a specific code:
//
//	errors.Is(Wrap(CodeNotFound, "tenant not found", cause), ErrNotFound) // true
func (e *CFError) Is(target error) bool {
	t, ok := target.(*CFError)
	if !ok {
		return false
	}
	return e.code == t.code
}

// Sentinel errors for use with errors.Is across layer boundaries.
// Each layer may wrap these sentinels with more specific messages; the
// code comparison in Is() ensures errors.Is still resolves correctly.
var (
	ErrNotFound      = New(CodeNotFound, "not found")
	ErrAlreadyExists = New(CodeAlreadyExists, "already exists")
	ErrInvalidInput  = New(CodeInvalidInput, "invalid input")
	ErrUnauthorized  = New(CodeUnauthorized, "unauthorized")
	ErrForbidden     = New(CodeForbidden, "forbidden")
	ErrInternal      = New(CodeInternal, "internal error")
	ErrUnavailable   = New(CodeUnavailable, "service unavailable")
	ErrConflict      = New(CodeConflict, "conflict")
)
