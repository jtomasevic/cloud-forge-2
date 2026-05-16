package service

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrAccountNotFound    = cferrors.New(cferrors.CodeNotFound, "account not found")
	ErrAccountEmailTaken  = cferrors.New(cferrors.CodeAlreadyExists, "email already registered")
	ErrInvalidCredentials = cferrors.New(cferrors.CodeUnauthorized, "invalid email or password")
	ErrTenantNotFound     = cferrors.New(cferrors.CodeNotFound, "tenant not found")
	ErrNetworkNotFound    = cferrors.New(cferrors.CodeNotFound, "network not found")
	ErrCredentialNotFound = cferrors.New(cferrors.CodeNotFound, "credential not found")
	ErrCredentialRevoked  = cferrors.New(cferrors.CodeForbidden, "API key has been revoked")
	ErrSlugConflict       = cferrors.New(cferrors.CodeConflict, "tenant slug conflict")
	ErrResolutionFailed   = cferrors.New(cferrors.CodeUnauthorized, "cannot resolve tenant from provided credentials")
)
