package credentials

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrCredentialNotFound = cferrors.New(cferrors.CodeNotFound, "API key not found")
	ErrCredentialRevoked  = cferrors.New(cferrors.CodeForbidden, "API key has been revoked")
)
