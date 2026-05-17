package vcluster

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrVClusterNotFound   = cferrors.New(cferrors.CodeNotFound, "vcluster not found")
	ErrVClusterExists     = cferrors.New(cferrors.CodeAlreadyExists, "vcluster already exists")
	ErrVClusterNotReady   = cferrors.New(cferrors.CodeUnavailable, "vcluster not ready")
	ErrKubeconfigNotReady = cferrors.New(cferrors.CodeUnavailable, "kubeconfig not available yet")
)
