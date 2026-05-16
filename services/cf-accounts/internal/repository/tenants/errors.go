package tenants

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrTenantNotFound = cferrors.New(cferrors.CodeNotFound, "tenant not found")
	ErrSlugTaken      = cferrors.New(cferrors.CodeAlreadyExists, "tenant slug already in use")
)
