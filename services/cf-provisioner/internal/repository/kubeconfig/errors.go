package kubeconfig

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var ErrKubeconfigNotFound = cferrors.New(cferrors.CodeNotFound, "kubeconfig not found — may have been revoked")
