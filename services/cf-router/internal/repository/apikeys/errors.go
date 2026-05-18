package apikeys

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

// Repository sentinel errors (typed *cferrors.CFError for consistent REST mapping).
var (
	// ErrKeyNotFound means no row exists for the BLAKE2b-256 hex digest (wrong key).
	ErrKeyNotFound = cferrors.New(cferrors.CodeUnauthorized, "API key not found")
	// ErrKeyRevoked means the hash row exists but revoked_at is non-null / non-zero.
	ErrKeyRevoked = cferrors.New(cferrors.CodeForbidden, "API key has been revoked")
)
