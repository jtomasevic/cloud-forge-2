package subnets

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrSubnetNotFound   = cferrors.New(cferrors.CodeNotFound, "subnet not found")
	ErrSubnetCIDRExists = cferrors.New(cferrors.CodeAlreadyExists, "subnet CIDR already exists in network")
)
