package accounts

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrAccountNotFound = cferrors.New(cferrors.CodeNotFound, "account not found")
	ErrAccountExists   = cferrors.New(cferrors.CodeAlreadyExists, "account with this email already exists")
)
