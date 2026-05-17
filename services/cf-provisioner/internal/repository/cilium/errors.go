package cilium

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrPolicyNotFound    = cferrors.New(cferrors.CodeNotFound, "cilium policy not found")
	ErrPolicyApplyFailed = cferrors.New(cferrors.CodeInternal, "failed to apply cilium policy")
)
