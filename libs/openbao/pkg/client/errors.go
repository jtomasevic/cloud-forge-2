package client

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

// ── Sentinel errors ───────────────────────────────────────────────────────────
// All errors returned by this package are defined here. Call sites use
// cferrors.Wrapf to attach runtime context while keeping errors.Is working.

var (
	// ErrSecretNotFound is returned when the requested vault path does not exist
	// or has been deleted.
	ErrSecretNotFound = cferrors.New(cferrors.CodeNotFound, "secret not found")

	// ErrClientInit is returned when the OpenBao client cannot be initialised
	// (empty address, malformed URL, TLS configuration failure).
	ErrClientInit = cferrors.New(cferrors.CodeInternal, "openbao client initialization failed")

	// ErrVaultWrite is returned when a write operation to vault fails.
	ErrVaultWrite = cferrors.New(cferrors.CodeInternal, "failed to write secret to vault")

	// ErrVaultCorrupt is returned when a secret read from vault has an
	// unexpected structure: missing keys, wrong value types, or invalid encoding.
	ErrVaultCorrupt = cferrors.New(cferrors.CodeInternal, "secret data in vault is malformed")

	// ErrVaultDelete is returned when a delete operation on vault fails.
	ErrVaultDelete = cferrors.New(cferrors.CodeInternal, "failed to delete secret from vault")
)
