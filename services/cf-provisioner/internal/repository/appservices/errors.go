package appservices

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrAppServiceNotFound = cferrors.New(cferrors.CodeNotFound, "app service not found")
	ErrAppServiceExists   = cferrors.New(cferrors.CodeAlreadyExists, "app service already exists")
	ErrInvalidAppService  = cferrors.New(cferrors.CodeInvalidInput, "invalid app service")
)
