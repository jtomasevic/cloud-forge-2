package cidr

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrCIDRExhausted        = cferrors.New(cferrors.CodeUnavailable, "platform CIDR pool exhausted")
	ErrCIDRAlreadyAllocated = cferrors.New(cferrors.CodeAlreadyExists, "network already has a CIDR allocation")
	ErrCIDRNotFound         = cferrors.New(cferrors.CodeNotFound, "CIDR allocation not found")
	ErrCIDRConflict         = cferrors.New(cferrors.CodeConflict, "requested CIDR overlaps with existing allocation")
)
