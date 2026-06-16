package workloads

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrWorkloadNotFound     = cferrors.New(cferrors.CodeNotFound, "workload not found")
	ErrInvalidWorkload      = cferrors.New(cferrors.CodeInvalidInput, "invalid workload")
	ErrWorkloadApplyFailed  = cferrors.New(cferrors.CodeInternal, "failed to apply workload")
	ErrWorkloadDeleteFailed = cferrors.New(cferrors.CodeInternal, "failed to delete workload")
)

func invalidWorkload(message string) error {
	return cferrors.Wrap(ErrInvalidWorkload.Code(), message, ErrInvalidWorkload)
}
