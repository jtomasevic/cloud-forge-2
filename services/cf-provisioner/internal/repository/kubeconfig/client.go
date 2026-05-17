package kubeconfig

import (
	"context"
	"errors"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	openbao "github.com/jtomasevic/cloud-forge-2/libs/openbao/pkg/client"
)

type cfKubeconfigRepository struct {
	secrets openbao.SecretsClient
}

func (r *cfKubeconfigRepository) Store(ctx context.Context, tenantID string, kubeconfigBytes []byte) error {
	if r.secrets == nil {
		return cferrors.Wrap(cferrors.CodeInternal, "secrets client is nil", cferrors.ErrInternal)
	}
	return openbao.StoreKubeconfig(ctx, r.secrets, tenantID, kubeconfigBytes)
}

func (r *cfKubeconfigRepository) Load(ctx context.Context, tenantID string) ([]byte, error) {
	if r.secrets == nil {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "secrets client is nil", cferrors.ErrInternal)
	}
	data, err := openbao.LoadKubeconfig(ctx, r.secrets, tenantID)
	if err != nil {
		if errors.Is(err, openbao.ErrSecretNotFound) {
			return nil, cferrors.Wrapf(ErrKubeconfigNotFound, "tenant %s", tenantID)
		}
		return nil, err
	}
	return data, nil
}

func (r *cfKubeconfigRepository) Revoke(ctx context.Context, tenantID string) error {
	if r.secrets == nil {
		return cferrors.Wrap(cferrors.CodeInternal, "secrets client is nil", cferrors.ErrInternal)
	}
	return openbao.RevokeKubeconfig(ctx, r.secrets, tenantID)
}
