package identity

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrUserExists           = cferrors.New(cferrors.CodeAlreadyExists, "identity user already exists")
	ErrUserNotFound         = cferrors.New(cferrors.CodeNotFound, "identity user not found")
	ErrAuthenticationFailed = cferrors.New(cferrors.CodeUnauthorized, "identity authentication failed")
	ErrClientConfig         = cferrors.New(cferrors.CodeInternal, "identity provider configuration is invalid")
	ErrIdentityService      = cferrors.New(cferrors.CodeUnavailable, "identity provider request failed")
)
