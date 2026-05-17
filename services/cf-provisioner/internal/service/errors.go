package service

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrNetworkNotFound    = cferrors.New(cferrors.CodeNotFound, "network not found")
	ErrNetworkNotActive   = cferrors.New(cferrors.CodeConflict, "network is not in active state")
	ErrJobNotFound        = cferrors.New(cferrors.CodeNotFound, "provisioning job not found")
	ErrGatewayNotFound    = cferrors.New(cferrors.CodeNotFound, "gateway not found")
	ErrGatewayExists      = cferrors.New(cferrors.CodeAlreadyExists, "gateway already provisioned for this network")
	ErrProvisioningFailed = cferrors.New(cferrors.CodeProvisioningFailed, "provisioning operation failed")
)
